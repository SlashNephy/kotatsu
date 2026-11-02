import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { contextMenuItems, usePlayerContextMenu } from './PlayerContextMenu';
import type { PlayerCommands } from './commands';

afterEach(() => {
  cleanup();
});

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

function Player({ commands }: { commands: PlayerCommands }) {
  const { onContextMenu, menu } = usePlayerContextMenu(commands);

  return (
    <div onContextMenu={onContextMenu}>
      <video data-testid="video" />
      {menu}
    </div>
  );
}

describe('usePlayerContextMenu', () => {
  test('閉じても、メニューの位置を右クリックした所に留める', () => {
    // 閉じる間のアニメーションで、メニューが左上へ動いて見えないようにする (#1259)。
    const { container } = render(
      <MantineProvider env="test">
        <Player commands={{ toggleMute: () => {} }} />
      </MantineProvider>,
    );
    fireEvent.contextMenu(screen.getByTestId('video'), { clientX: 120, clientY: 80 });
    const anchor = container.querySelector<HTMLElement>('[aria-hidden="true"]');
    expect(anchor?.style.left).toBe('120px');
    expect(screen.getByRole('menu')).toBeDefined();

    fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' });
    expect(screen.queryByRole('menu')).toBeNull();
    expect([anchor?.style.left, anchor?.style.top]).toEqual(['120px', '80px']);
  });
});
