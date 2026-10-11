import { describe, expect, test } from 'vitest';

import { fitContain, overlayCanvases } from './capture';

describe('fitContain', () => {
  test.each([
    {
      name: '同じ比率なら、枠いっぱい',
      size: [1280, 720],
      want: { x: 0, y: 0, width: 1920, height: 1080 },
    },
    {
      name: '4:3 は、左右に黒い帯を残す',
      size: [640, 480],
      want: { x: 240, y: 0, width: 1440, height: 1080 },
    },
    {
      name: '大きさが分からなければ、描かない',
      size: [0, 0],
      want: { x: 0, y: 0, width: 0, height: 0 },
    },
  ])('$name', ({ size, want }) => {
    expect(fitContain(size[0] ?? 0, size[1] ?? 0, 1920, 1080)).toEqual(want);
  });
});

describe('overlayCanvases', () => {
  test('印の付いた、出している canvas だけを返し、yadif の canvas は除く', () => {
    const frame = document.createElement('div');
    const media = document.createElement('div');
    const video = document.createElement('video');
    const yadif = document.createElement('canvas');
    const captions = document.createElement('canvas');
    captions.dataset.overlay = 'captions';
    const hidden = document.createElement('canvas');
    hidden.dataset.overlay = 'captions';
    hidden.style.visibility = 'hidden';
    const comments = document.createElement('canvas');
    comments.dataset.overlay = 'comments';
    // kyusu の経路では、字幕の canvas が video と同じ親に入る。
    media.append(video, yadif, captions, hidden);
    frame.append(media, comments);

    expect(overlayCanvases(frame)).toEqual([captions, comments]);
  });
});
