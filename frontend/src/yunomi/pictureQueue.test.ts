import { describe, expect, test } from 'vitest';

import { PictureQueue, type QueuedPicture } from './pictureQueue';

// 29.97 コマ/秒の 1 コマは、PTS で 3003。
const frame = 3003;

function picture(pts: number, topFieldFirst = true, interlaced = true): QueuedPicture<string> {
  return { pts, interlaced, topFieldFirst, slot: `s${pts}` };
}

function setup() {
  const released: string[] = [];
  const queue = new PictureQueue<string>((slot) => released.push(slot));

  return { queue, released };
}

describe('PictureQueue', () => {
  test('コマの前半は 1 枚目、後半は 2 枚目のフィールドを映す', () => {
    const { queue } = setup();
    queue.push(picture(0));
    queue.push(picture(frame));

    const choices = [0, 1500, 1502, 3000].map((target) => {
      const choice = queue.choose(target);

      return choice === null ? null : [choice.picture.pts, choice.second, choice.parity];
    });

    expect(choices).toEqual([
      [0, false, 0],
      [0, false, 0],
      [0, true, 1],
      [0, true, 1],
    ]);
  });

  test('ボトムフィールドが先のコマは、下のフィールドから映す', () => {
    const { queue } = setup();
    queue.push(picture(0, false));
    queue.push(picture(frame, false));

    expect([queue.choose(0)?.parity, queue.choose(2000)?.parity]).toEqual([1, 0]);
  });

  test('プログレッシブのコマは、コマ全体を映す', () => {
    const { queue } = setup();
    queue.push(picture(0, true, false));
    queue.push(picture(frame, true, false));

    expect([queue.choose(2000)?.second, queue.choose(2000)?.parity]).toEqual([false, 0]);
  });

  test('番を過ぎたコマは映さずに捨て、コマ落ちとして数える。映したコマは前のコマとして残す', () => {
    const { queue, released } = setup();
    for (let i = 0; i < 4; i++) {
      queue.push(picture(i * frame));
    }
    queue.choose(0);
    const choice = queue.choose(3 * frame);

    expect(choice?.picture.pts).toBe(3 * frame);
    expect(queue.previous?.pts).toBe(0);
    expect(queue.dropped).toBe(2);
    expect(released).toEqual([`s${frame}`, `s${2 * frame}`]);
  });

  test('まだ番の来ていないコマは映さない', () => {
    const { queue } = setup();
    queue.push(picture(frame));

    expect(queue.choose(frame - 1)).toBeNull();
  });

  test('前より古い時刻のコマは足さずに捨てる', () => {
    const { queue, released } = setup();
    queue.push(picture(frame));

    expect(queue.push(picture(0))).toBe(false);
    expect(released).toEqual(['s0']);
    expect(queue.length).toBe(1);
  });

  test('clear で、待っているコマと前のコマの slot を返す', () => {
    const { queue, released } = setup();
    queue.push(picture(0));
    queue.push(picture(frame));
    queue.push(picture(2 * frame));
    queue.choose(0);
    queue.choose(frame);
    queue.clear();

    expect(released.toSorted()).toEqual(['s0', `s${frame}`, `s${2 * frame}`].toSorted());
    expect(queue.length).toBe(0);
    expect(queue.newest).toBeNull();
  });
});
