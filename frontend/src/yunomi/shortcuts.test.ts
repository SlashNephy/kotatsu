import { describe, expect, test } from 'vitest';

import {
  assignKey,
  defaultKeymap,
  eventKey,
  keysFor,
  parseKeymap,
  removeKey,
  shortcutFor,
} from './shortcuts';

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

describe('shortcutFor with a keymap', () => {
  test('割り当てを変えたら、そのキーで操作を呼ぶ', () => {
    const keymap = assignKey(defaultKeymap, 'toggleMute', 'x');
    const event = {
      key: 'x',
      ctrlKey: false,
      metaKey: false,
      altKey: false,
      shiftKey: false,
      repeat: false,
      target: document.body,
    };

    expect(shortcutFor(event, keymap)).toBe('toggleMute');
  });
});

describe('eventKey', () => {
  const event = (
    key: string,
    mods: Partial<{ ctrlKey: boolean; altKey: boolean; metaKey: boolean; shiftKey: boolean }> = {},
  ) => ({
    key,
    ctrlKey: false,
    altKey: false,
    metaKey: false,
    shiftKey: false,
    ...mods,
  });

  test.each([
    { name: '文字のキーは、そのまま', event: event('x'), want: 'x' },
    {
      name: 'Shift で変わった文字は、変わった文字のまま',
      event: event('?', { shiftKey: true }),
      want: '?',
    },
    {
      name: '文字でないキーと Shift は、Shift+ を付ける',
      event: event('ArrowLeft', { shiftKey: true }),
      want: 'Shift+ArrowLeft',
    },
    { name: '修飾キーだけなら null', event: event('Shift', { shiftKey: true }), want: null },
    { name: 'Ctrl と一緒なら null', event: event('x', { ctrlKey: true }), want: null },
    { name: 'Escape は、割り当てをやめるのに使うので null', event: event('Escape'), want: null },
  ])('$name', ({ event, want }) => {
    expect(eventKey(event)).toBe(want);
  });
});

describe('assignKey', () => {
  test('ほかの操作に割り当てていたキーは、そちらから外す', () => {
    const keymap = assignKey(defaultKeymap, 'toggleMute', 'c');

    expect(keysFor('toggleMute', keymap)).toEqual(['m', 'c']);
    expect(keysFor('toggleCaptions', keymap)).toEqual([]);
  });

  test('同じキーを 2 度足さない', () => {
    expect(keysFor('toggleMute', assignKey(defaultKeymap, 'toggleMute', 'm'))).toEqual(['m']);
  });
});

describe('removeKey', () => {
  test('操作からキーを外す', () => {
    expect(keysFor('togglePlay', removeKey(defaultKeymap, 'togglePlay', 'k'))).toEqual([' ']);
  });
});

describe('parseKeymap', () => {
  test.each([
    {
      name: '覚えた割り当てを読む',
      raw: '{"toggleMute":["x"]}',
      want: { ...defaultKeymap, toggleMute: ['x'] },
    },
    {
      name: '知らない操作と、文字列でないキーは捨てる',
      raw: '{"unknown":["x"],"toggleMute":[1,"y"]}',
      want: { ...defaultKeymap, toggleMute: ['y'] },
    },
    { name: '壊れていれば既定', raw: '{', want: defaultKeymap },
    { name: 'なければ既定', raw: null, want: defaultKeymap },
  ])('$name', ({ raw, want }) => {
    expect(parseKeymap(raw)).toEqual(want);
  });
});
