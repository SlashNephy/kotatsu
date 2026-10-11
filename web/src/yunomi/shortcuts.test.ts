import { describe, expect, test } from 'vitest';

import { shortcutFor } from './shortcuts';

describe('shortcutFor', () => {
  const body = document.body;
  const input = document.createElement('input');

  test.each([
    {
      name: 'スペースで再生と一時停止',
      key: ' ',
      target: body,
      ctrlKey: false,
      want: 'togglePlay',
    },
    {
      name: '左の矢印で戻る',
      key: 'ArrowLeft',
      target: body,
      ctrlKey: false,
      want: 'seekBackward',
    },
    { name: 'l で進む', key: 'l', target: body, ctrlKey: false, want: 'seekForward' },
    { name: 'c で字幕', key: 'c', target: body, ctrlKey: false, want: 'toggleCaptions' },
    { name: '入力欄の中では使わない', key: ' ', target: input, ctrlKey: false, want: null },
    { name: '修飾キーと一緒なら使わない', key: 'c', target: body, ctrlKey: true, want: null },
    { name: '割り当てのないキー', key: 'x', target: body, ctrlKey: false, want: null },
  ])('$name', ({ key, target, ctrlKey, want }) => {
    expect(shortcutFor({ key, target, ctrlKey, metaKey: false, altKey: false })).toBe(want);
  });
});
