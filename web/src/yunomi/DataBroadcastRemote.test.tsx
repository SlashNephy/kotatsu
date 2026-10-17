import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { theme } from '../futon/theme';
import {
  DataBroadcastRemote,
  DataBroadcastRemoteOverlay,
  repeatDelayMs,
  repeatIntervalMs,
} from './DataBroadcastRemote';
import { onVideoSurface } from './useControlsVisibility';
import type { DataBroadcastState, KeyGroup } from './useDataBroadcast';

afterEach(cleanup);

const allGroups: ReadonlySet<KeyGroup> = new Set(['basic', 'data-button', 'numeric-tuning']);

function renderRemote(
  state: DataBroadcastState,
  receiving = false,
  keyGroups: ReadonlySet<KeyGroup> = allGroups,
) {
  const onPress = vi.fn();
  render(
    <MantineProvider theme={theme}>
      <DataBroadcastRemote
        state={state}
        receiving={receiving}
        keyGroups={keyGroups}
        onPress={onPress}
      />
    </MantineProvider>,
  );

  return onPress;
}

describe('DataBroadcastRemote', () => {
  // 値は web-bml の AribKeyCode。
  test.each([
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

  test('矢印を押し続けると繰り返し、離したら止まる。離した後の click では、もう押さない', () => {
    vi.useFakeTimers();
    try {
      const onPress = renderRemote('shown');
      const down = screen.getByRole('button', { name: '下' });

      fireEvent.pointerDown(down, { button: 0 });
      expect(onPress).toHaveBeenCalledTimes(1);
      vi.advanceTimersByTime(repeatDelayMs - 1);
      expect(onPress).toHaveBeenCalledTimes(1);
      vi.advanceTimersByTime(1 + repeatIntervalMs * 2);
      expect(onPress).toHaveBeenCalledTimes(4);
      fireEvent.pointerUp(down, { button: 0 });
      fireEvent.click(down, { detail: 1 });
      vi.advanceTimersByTime(repeatIntervalMs * 5);
      expect(onPress).toHaveBeenCalledTimes(4);
      expect(onPress).toHaveBeenLastCalledWith(2);
    } finally {
      vi.useRealTimers();
    }
  });

  test('キーボードで矢印のボタンを押すと (detail が 0 の click だけが届く)、1 回押す', () => {
    const onPress = renderRemote('shown');

    fireEvent.click(screen.getByRole('button', { name: '上' }), { detail: 0 });

    expect(onPress).toHaveBeenCalledExactlyOnceWith(1);
  });

  test('タッチで矢印のボタンを叩くと (pointerleave が click より先に届く)、1 回だけ押す', () => {
    const onPress = renderRemote('shown');
    const up = screen.getByRole('button', { name: '上' });

    fireEvent.pointerDown(up, { button: 0 });
    fireEvent.pointerUp(up, { button: 0 });
    fireEvent.pointerLeave(up);
    fireEvent.click(up, { detail: 1 });

    expect(onPress).toHaveBeenCalledExactlyOnceWith(1);
  });

  // 受け取っている間の知らせは、映像の右下に出すので、リモコンには出さない。
  test('受け取っている間は、ボタンを押せない', () => {
    renderRemote('loading');

    expect(screen.getByRole<HTMLButtonElement>('button', { name: '決定' }).disabled).toBe(true);
  });

  test('文書を読んでいても、d ボタンで入っている途中は、ボタンを押せない', () => {
    renderRemote('shown', true);

    expect(screen.getByRole<HTMLButtonElement>('button', { name: '決定' }).disabled).toBe(true);
  });

  test('受け取れなくなったら、ボタンを押せず、理由を出す', () => {
    renderRemote('ended');

    expect(screen.getByRole<HTMLButtonElement>('button', { name: '決定' }).disabled).toBe(true);
    expect(screen.queryByText('データ放送を受け取れなくなりました')).not.toBeNull();
  });

  test.each([
    { button: '数字 1', want: 6 },
    { button: '数字 9', want: 14 },
    { button: '数字 12', want: 17 },
    { button: '数字 0', want: 5 },
  ])('数字を開いて $button を押すと、web-bml のキー $want を送る', async ({ button, want }) => {
    const onPress = renderRemote('shown');

    await userEvent.click(screen.getByRole('button', { name: '数字' }));
    await userEvent.click(screen.getByRole('button', { name: button }));

    expect(onPress).toHaveBeenCalledExactlyOnceWith(want);
  });

  test.each([
    {
      name: '文書が basic だけを使うなら、色と数字のボタンは押せない',
      groups: ['basic'],
      enabled: { 決定: true, 青: false, '数字 1': false },
    },
    {
      name: '文書が data-button だけを使うなら、色のボタンだけを押せる',
      groups: ['data-button'],
      enabled: { 決定: false, 青: true, '数字 1': false },
    },
    {
      name: '文書が numeric-tuning も使うなら、数字のボタンを押せる',
      groups: ['basic', 'numeric-tuning'],
      enabled: { 決定: true, 青: false, '数字 1': true },
    },
  ] satisfies { name: string; groups: KeyGroup[]; enabled: Record<string, boolean> }[])(
    '$name',
    async ({ groups, enabled }) => {
      renderRemote('shown', false, new Set(groups));
      await userEvent.click(screen.getByRole('button', { name: '数字' }));

      for (const [button, want] of Object.entries(enabled)) {
        expect(screen.getByRole<HTMLButtonElement>('button', { name: button }).disabled).toBe(
          !want,
        );
      }
    },
  );
});

describe('DataBroadcastRemoteOverlay', () => {
  test('閉じた状態で始まり、開くとリモコンを出し、押したキーを渡す', async () => {
    const onPress = vi.fn();
    render(
      <MantineProvider theme={theme}>
        <DataBroadcastRemoteOverlay
          state="shown"
          receiving={false}
          keyGroups={allGroups}
          onPress={onPress}
        />
      </MantineProvider>,
    );

    expect(screen.queryByRole('group', { name: 'リモコン' })).toBeNull();
    await userEvent.click(screen.getByRole('button', { name: 'リモコンを出す' }));
    await userEvent.click(screen.getByRole('button', { name: '決定' }));
    expect(onPress).toHaveBeenCalledWith(18);

    await userEvent.click(screen.getByRole('button', { name: 'リモコンを隠す' }));
    expect(screen.queryByRole('group', { name: 'リモコン' })).toBeNull();
  });

  test('リモコンの中を押しても、映像を押したことにはしない', async () => {
    render(
      <MantineProvider theme={theme}>
        <DataBroadcastRemoteOverlay
          state="shown"
          receiving={false}
          keyGroups={allGroups}
          onPress={vi.fn()}
        />
      </MantineProvider>,
    );
    await userEvent.click(screen.getByRole('button', { name: 'リモコンを出す' }));

    // ボタンの間の余白 (パネル) を押しても、再生や全画面を切り替えない。
    const panel = screen.getByRole('group', { name: 'リモコン' }).parentElement;
    expect(onVideoSurface(panel)).toBe(false);
  });
});
