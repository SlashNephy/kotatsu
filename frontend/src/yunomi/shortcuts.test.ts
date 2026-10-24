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
    {
      name: 'Shift と左の矢印で大きく戻る',
      key: 'ArrowLeft',
      target: body,
      ctrlKey: false,
      shiftKey: true,
      want: 'seekBackwardLong',
    },
    {
      name: 'Shift と右の矢印を押し続けると、大きく進むのを繰り返す',
      key: 'ArrowRight',
      target: body,
      ctrlKey: false,
      shiftKey: true,
      repeat: true,
      want: 'seekForwardLong',
    },
    {
      name: 'Shift の割り当てがなければ、Shift のないキーの操作にする',
      key: 'ArrowUp',
      target: body,
      ctrlKey: false,
      shiftKey: true,
      want: 'volumeUp',
    },
    { name: 'l で進む', key: 'l', target: body, ctrlKey: false, want: 'seekForward' },
    { name: 'c で字幕', key: 'c', target: body, ctrlKey: false, want: 'toggleCaptions' },
    {
      name: '? でショートカットの一覧',
      key: '?',
      target: body,
      ctrlKey: false,
      want: 'showShortcuts',
    },
    {
      name: '上の矢印で音量を上げる',
      key: 'ArrowUp',
      target: body,
      ctrlKey: false,
      want: 'volumeUp',
    },
    { name: 'd でデータ放送', key: 'd', target: body, ctrlKey: false, want: 'toggleDataBroadcast' },
    {
      name: '押し続けると、音量は繰り返し変える',
      key: 'ArrowDown',
      target: body,
      ctrlKey: false,
      repeat: true,
      want: 'volumeDown',
    },
    { name: '入力欄の中では使わない', key: ' ', target: input, ctrlKey: false, want: null },
    { name: '修飾キーと一緒なら使わない', key: 'c', target: body, ctrlKey: true, want: null },
    { name: '割り当てのないキー', key: 'x', target: body, ctrlKey: false, want: null },
    {
      name: '押し続けても、切り替えは繰り返さない',
      key: ' ',
      target: body,
      ctrlKey: false,
      repeat: true,
      want: null,
    },
    {
      name: '押し続けると、シークは繰り返す',
      key: 'ArrowRight',
      target: body,
      ctrlKey: false,
      repeat: true,
      want: 'seekForward',
    },
  ])('$name', ({ key, target, ctrlKey, shiftKey = false, repeat = false, want }) => {
    expect(
      shortcutFor({ key, target, ctrlKey, metaKey: false, altKey: false, shiftKey, repeat }),
    ).toBe(want);
  });
});
