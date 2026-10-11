import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { theme } from '../futon/theme';
import { DataBroadcastRemote } from './DataBroadcastRemote';
import type { DataBroadcastState } from './useDataBroadcast';

afterEach(cleanup);

function renderRemote(state: DataBroadcastState) {
  const onPress = vi.fn();
  render(
    <MantineProvider theme={theme}>
      <DataBroadcastRemote state={state} onPress={onPress} />
    </MantineProvider>,
  );

  return onPress;
}

describe('DataBroadcastRemote', () => {
  // 値は web-bml の AribKeyCode。
  test.each([
    { button: 'd ボタン', want: 20 },
    { button: '上', want: 1 },
    { button: '下', want: 2 },
    { button: '左', want: 3 },
    { button: '右', want: 4 },
    { button: '決定', want: 18 },
    { button: '戻る', want: 19 },
    { button: '青', want: 21 },
    { button: '赤', want: 22 },
    { button: '緑', want: 23 },
    { button: '黄', want: 24 },
  ])('$button を押すと、web-bml のキー $want を送る', async ({ button, want }) => {
    const onPress = renderRemote('shown');

    await userEvent.click(screen.getByRole('button', { name: button }));

    expect(onPress).toHaveBeenCalledExactlyOnceWith(want);
  });

  test.each([
    { state: 'loading' as const, message: 'データ放送を受け取っています' },
    { state: 'ended' as const, message: 'データ放送を受け取れなくなりました' },
  ])('$state の間は、ボタンを押せず、理由を出す', ({ state, message }) => {
    renderRemote(state);

    expect(screen.getByRole<HTMLButtonElement>('button', { name: '決定' }).disabled).toBe(true);
    expect(screen.queryByText(message)).not.toBeNull();
  });
});
