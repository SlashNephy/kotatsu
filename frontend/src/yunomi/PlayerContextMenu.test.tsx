import { describe, expect, test } from 'vitest';

import { contextMenuItems } from './PlayerContextMenu';

describe('contextMenuItems', () => {
  test('今使える、キーに割り当てられる操作を、登録簿の順に並べる', () => {
    const noop = () => {};

    expect(
      contextMenuItems({
        toggleMute: noop,
        togglePlay: noop,
        setVolume: noop,
        toggleFullscreen: noop,
      }),
    ).toEqual(['togglePlay', 'toggleMute', 'toggleFullscreen']);
  });

  test('再生と一時停止は、切り替えがあるので並べない', () => {
    const noop = () => {};

    expect(contextMenuItems({ play: noop, pause: noop, togglePlay: noop })).toEqual(['togglePlay']);
  });
});
