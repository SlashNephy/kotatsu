import { describe, expect, test } from 'vitest';

import { isSurroundAudio } from './useVirtualSurround';
import { position } from './virtualSurround';

describe('position', () => {
  test.each([
    { name: '正面', azimuth: 0, want: { x: 0, z: -1 } },
    { name: '右', azimuth: 90, want: { x: 1, z: 0 } },
    { name: '左後ろ (サラウンドの左)', azimuth: -110, want: { x: -0.9397, z: 0.342 } },
  ])('$name', ({ azimuth, want }) => {
    const got = position(azimuth);
    expect(got.x).toBeCloseTo(want.x, 3);
    expect(got.z).toBeCloseTo(want.z, 3);
  });
});

describe('isSurroundAudio', () => {
  test.each([
    { name: '主音声が 5.1ch', audios: [{ componentType: 9 }, { componentType: 3 }], want: true },
    { name: '主音声がステレオ', audios: [{ componentType: 3 }, { componentType: 9 }], want: false },
    { name: '音声が分からない', audios: null, want: false },
  ])('$name', ({ audios, want }) => {
    expect(isSurroundAudio(audios)).toBe(want);
  });
});
