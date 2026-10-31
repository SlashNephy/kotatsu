import { describe, expect, test } from 'vitest';

import {
  assignButton,
  buttonLabel,
  commandForButton,
  defaultGamepadMap,
  newlyPressed,
  parseGamepadMap,
  repeatDue,
} from './gamepad';

describe('newlyPressed', () => {
  test('前に押していなかったボタンだけを返す', () => {
    expect(newlyPressed([true, false, false], [true, true, false])).toEqual([1]);
  });

  test('前の状態がなければ、押しているボタンを全部返す', () => {
    expect(newlyPressed([], [false, true, true])).toEqual([1, 2]);
  });
});

describe('commandForButton', () => {
  test.each([
    { name: 'A で再生と一時停止', button: 0, want: 'togglePlay' },
    { name: '十字キーの左で戻る', button: 14, want: 'seekBackward' },
    { name: '割り当てのないボタン', button: 16, want: null },
  ])('$name', ({ button, want }) => {
    expect(commandForButton(defaultGamepadMap, button)).toBe(want);
  });
});

describe('repeatDue', () => {
  test.each([
    { name: '押し始めてすぐは繰り返さない', heldSince: 0, lastFired: 0, now: 300, want: false },
    { name: '押し続けたら繰り返す', heldSince: 0, lastFired: 0, now: 450, want: true },
    { name: '繰り返した直後は待つ', heldSince: 0, lastFired: 450, now: 500, want: false },
    { name: '間を空けたら、また繰り返す', heldSince: 0, lastFired: 450, now: 620, want: true },
  ])('$name', ({ heldSince, lastFired, now, want }) => {
    expect(repeatDue(heldSince, lastFired, now)).toBe(want);
  });
});

describe('assignButton', () => {
  test('ほかの操作に割り当てていたボタンは、そちらから外す', () => {
    const map = assignButton(defaultGamepadMap, 'toggleMute', 0);

    expect(commandForButton(map, 0)).toBe('toggleMute');
    expect(map.togglePlay).toEqual([]);
  });
});

describe('parseGamepadMap', () => {
  test.each([
    { name: 'なければ既定', raw: null, want: defaultGamepadMap },
    { name: '壊れていれば既定', raw: '{', want: defaultGamepadMap },
    {
      name: '覚えた割り当てを読み、数でないものは捨てる',
      raw: '{"toggleMute":[1,"x"],"togglePlay":[]}',
      want: { ...defaultGamepadMap, toggleMute: [1], togglePlay: [] },
    },
  ])('$name', ({ raw, want }) => {
    expect(parseGamepadMap(raw)).toEqual(want);
  });
});

describe('buttonLabel', () => {
  test.each([
    { button: 0, want: 'A' },
    { button: 12, want: '↑' },
    { button: 20, want: '20' },
  ])('$button', ({ button, want }) => {
    expect(buttonLabel(button)).toBe(want);
  });
});
