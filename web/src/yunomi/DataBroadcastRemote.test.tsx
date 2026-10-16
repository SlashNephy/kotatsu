import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { theme } from '../futon/theme';
import { DataBroadcastRemote } from './DataBroadcastRemote';
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
