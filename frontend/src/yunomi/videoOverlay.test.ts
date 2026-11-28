import { describe, expect, test } from 'vitest';

import { containSize, deviceBox } from './videoOverlay';

describe('containSize', () => {
  test.each([
    {
      name: '同じ比率の枠なら、枠いっぱいに映す',
      picture: { width: 1920, height: 1080 },
      box: { width: 3840, height: 2160 },
      want: { width: 3840, height: 2160 },
    },
    {
      // 16:9 の映像を 4:3 の枠に映すと、上下に黒い帯が出る。
      name: '縦長の枠なら、幅に合わせる',
      picture: { width: 1920, height: 1080 },
      box: { width: 1440, height: 1080 },
      want: { width: 1440, height: 810 },
    },
    {
      // 16:9 の映像を、ウィンドウに合わせた横長の枠に映すと、左右に黒い帯が出る。
      name: '横長の枠なら、高さに合わせる',
      picture: { width: 1920, height: 1080 },
      box: { width: 2560, height: 1080 },
      want: { width: 1920, height: 1080 },
    },
    {
      name: '枠が空なら、映す大きさはない',
      picture: { width: 1920, height: 1080 },
      box: { width: 0, height: 0 },
      want: null,
    },
    {
      name: 'フレームが届いていなければ、映す大きさはない',
      picture: { width: 0, height: 0 },
      box: { width: 1920, height: 1080 },
      want: null,
    },
  ])('$name', ({ picture, box, want }) => {
    expect(containSize(picture, box)).toStrictEqual(want);
  });
});

describe('deviceBox', () => {
  test.each([
    {
      name: '端末の画素の大きさが比率と合えば、そのまま使う',
      css: { width: 960, height: 540 },
      device: { inlineSize: 1921, blockSize: 1080 },
      ratio: 2,
      want: { width: 1921, height: 1080 },
    },
    {
      // 自動操作で画素の比率を差し替えると、端末の画素の大きさは CSS の大きさのまま届く。
      name: '比率と食い違えば、CSS の大きさに比率を掛ける',
      css: { width: 960, height: 540 },
      device: { inlineSize: 960, blockSize: 540 },
      ratio: 2,
      want: { width: 1920, height: 1080 },
    },
    {
      name: '端末の画素の大きさを通知しなければ、CSS の大きさに比率を掛ける',
      css: { width: 960.4, height: 540.2 },
      device: undefined,
      ratio: 1.5,
      want: { width: 1441, height: 810 },
    },
  ])('$name', ({ css, device, ratio, want }) => {
    expect(deviceBox(css, device, ratio)).toStrictEqual(want);
  });
});
