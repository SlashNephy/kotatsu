import { describe, expect, test } from 'vitest';

import { chooseAudioPid, routeDualMono, startsWithPce } from './decodeAudio';

describe('chooseAudioPid', () => {
  test.each([
    {
      name: '選んだ ES が番組にあれば、それを鳴らす',
      pids: [0x110, 0x111],
      preferred: 0x111,
      want: 0x111,
    },
    { name: '選んでいなければ、最初の ES', pids: [0x110, 0x111], preferred: null, want: 0x110 },
    { name: '選んだ ES が番組になければ、最初の ES', pids: [0x110], preferred: 0x111, want: 0x110 },
    { name: '音声のない番組', pids: [], preferred: 0x111, want: null },
  ])('$name', ({ pids, preferred, want }) => {
    expect(chooseAudioPid(pids, preferred)).toBe(want);
  });
});

describe('routeDualMono', () => {
  const main = new Float32Array([0.1, 0.2]);
  const sub = new Float32Array([0.3, 0.4]);

  test.each([
    { channel: 'main' as const, left: [0.1, 0.2], right: [0.1, 0.2] },
    { channel: 'sub' as const, left: [0.3, 0.4], right: [0.3, 0.4] },
    { channel: 'both' as const, left: [0.1, 0.2], right: [0.3, 0.4] },
  ])('$channel', ({ channel, left, right }) => {
    const routed = routeDualMono(main.slice(), sub.slice(), channel);

    expect([...routed.left]).toEqual(left.map(Math.fround));
    expect([...routed.right]).toEqual(right.map(Math.fround));
    // 左右は、別々の領域にする。worker からメインスレッドへ、両方を移して渡すため。
    expect(routed.left.buffer).not.toBe(routed.right.buffer);
  });
});

describe('startsWithPce', () => {
  // ADTS のヘッダー (protection_absent が 1 の 7 バイト) と、最初の要素の id_syn_ele (上位 3 ビット)。
  const frame = (protectionAbsent: boolean, element: number) => {
    const header = [0xff, protectionAbsent ? 0xf1 : 0xf0, 0x4c, 0x00, 0x00, 0x00, 0xfc];
    const crc = protectionAbsent ? [] : [0x00, 0x00];

    return new Uint8Array([...header, ...crc, element << 5, 0x00]);
  };

  test.each([
    { name: 'PCE で始まる', data: frame(true, 5), want: true },
    { name: 'SCE で始まる', data: frame(true, 0), want: false },
    { name: 'CRC の後の PCE', data: frame(false, 5), want: true },
    { name: '短すぎる', data: new Uint8Array([0xff, 0xf1]), want: false },
  ])('$name', ({ data, want }) => {
    expect(startsWithPce(data)).toBe(want);
  });
});
