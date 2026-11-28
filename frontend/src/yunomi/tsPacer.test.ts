import { describe, expect, test } from 'vitest';

import { TsPacer } from './tsPacer';

// packets は、10 パケットごとに、i × step 秒の PCR を入れた TS を作る。ほかのパケットは、ペイロードを空にしたパケット。
function packets(count: number, step: number, from = 1000): Uint8Array {
  const bytes = new Uint8Array(count * 188);
  for (let i = 0; i < count; i += 1) {
    const p = i * 188;
    bytes[p] = 0x47;
    if (i % 10 === 0) {
      bytes[p + 3] = 0x20;
      bytes[p + 4] = 183;
      bytes[p + 5] = 0x10;
      const base = Math.round((from + i * step) * 90_000);
      bytes[p + 6] = Math.floor(base / 2 ** 25) & 0xff;
      bytes[p + 7] = Math.floor(base / 2 ** 17) & 0xff;
      bytes[p + 8] = Math.floor(base / 2 ** 9) & 0xff;
      bytes[p + 9] = Math.floor(base / 2) & 0xff;
      bytes[p + 10] = (base % 2) << 7;
    } else {
      bytes[p + 3] = 0x10;
    }
  }

  return bytes;
}

describe('TsPacer', () => {
  test.each([
    // 10 パケットごとに 0.1 秒進む TS で、経過に 0.5 秒先までを足した所の PCR の手前までを渡す。
    { name: '読み始めは、最初の PCR から 0.5 秒先まで渡す', elapsed: 0, want: 60 },
    { name: '経過が進めば、その 0.5 秒先まで渡す', elapsed: 1, want: 160 },
    { name: '届いた分より先まで進めば、届いた分をすべて渡す', elapsed: 100, want: 200 },
  ])('$name', ({ elapsed, want }) => {
    const pushed: Uint8Array[] = [];
    const pacer = new TsPacer((bytes) => pushed.push(bytes));
    pacer.feed(packets(200, 0.01));

    pacer.release(elapsed);

    expect(pushed.reduce((sum, bytes) => sum + bytes.length, 0)).toBe(want * 188);
    expect(pacer.pendingBytes).toBe((200 - want) * 188);
  });

  test('止めている間は、経過が進まないので、それ以上渡さない', () => {
    const pushed: Uint8Array[] = [];
    const pacer = new TsPacer((bytes) => pushed.push(bytes));
    pacer.feed(packets(200, 0.01));

    pacer.release(0);
    pacer.release(0);

    expect(pushed).toHaveLength(1);
  });

  test('読み終えたら、ためている分をすべて渡す', () => {
    const pushed: Uint8Array[] = [];
    const pacer = new TsPacer((bytes) => pushed.push(bytes));
    pacer.feed(packets(200, 0.01));

    pacer.release(0);
    pacer.flush();

    expect(pushed.reduce((sum, bytes) => sum + bytes.length, 0)).toBe(200 * 188);
    expect(pacer.pendingBytes).toBe(0);
  });

  test('PCR が戻っても、進まなかったものとして数え、止まらない', () => {
    const pushed: Uint8Array[] = [];
    const pacer = new TsPacer((bytes) => pushed.push(bytes));
    // 0 から 0.99 秒まで進んだあと、PCR が 100 秒戻ってから、また 0.01 秒ずつ進む。
    pacer.feed(packets(100, 0.01, 1000));
    pacer.feed(packets(100, 0.01, 900));

    pacer.release(1.5);

    // 戻った後の PCR は、0.99 秒から数え直すので、1.5 + 0.5 秒までの分を渡す。
    expect(pushed.reduce((sum, bytes) => sum + bytes.length, 0)).toBe(200 * 188);
  });

  test('少しずつ届いても、ためた分を失わない', () => {
    const pushed: Uint8Array[] = [];
    const pacer = new TsPacer((bytes) => pushed.push(bytes));
    const all = packets(2000, 0.01);
    for (let i = 0; i < all.length; i += 1000) {
      pacer.feed(all.subarray(i, i + 1000));
      pacer.release(5);
    }
    pacer.flush();

    const got = new Uint8Array(pushed.reduce((sum, bytes) => sum + bytes.length, 0));
    let offset = 0;
    for (const bytes of pushed) {
      got.set(bytes, offset);
      offset += bytes.length;
    }
    expect(got).toEqual(all);
  });
});
