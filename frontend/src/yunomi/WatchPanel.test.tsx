import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, test } from 'vitest';

import { WatchPanel, type WatchPanelTab } from './WatchPanel';

afterEach(() => {
  cleanup();
});

const jikkyo: WatchPanelTab = { value: 'jikkyo', label: '実況', content: <p>コメント</p> };
const captions: WatchPanelTab = { value: 'captions', label: '字幕', content: <p>字幕の行</p> };

function panel(tabs: readonly WatchPanelTab[]) {
  return (
    <MantineProvider env="test">
      <WatchPanel label="右の列" tabs={tabs} />
    </MantineProvider>
  );
}

describe('WatchPanel', () => {
  test('選ぶまでは先頭の中身を出し、後から先頭に足された中身に移る', () => {
    // 録画の実況の過去ログは、読み込めてから先頭に足される。
    const { rerender } = render(panel([captions]));
    expect(screen.getByText('字幕の行')).toBeDefined();

    rerender(panel([jikkyo, captions]));
    expect(screen.getByText('コメント')).toBeDefined();
    expect(screen.queryByText('字幕の行')).toBeNull();
  });

  test('選んだ中身は、ほかの中身が足されても保つ', async () => {
    const { rerender } = render(panel([jikkyo, captions]));
    await userEvent.click(screen.getByText('字幕'));
    expect(screen.getByText('字幕の行')).toBeDefined();

    const other: WatchPanelTab = { value: 'other', label: 'ほか', content: <p>ほかの中身</p> };
    rerender(panel([other, jikkyo, captions]));
    expect(screen.getByText('字幕の行')).toBeDefined();
  });

  test('選んだ中身がなくなったら、先頭に戻る', async () => {
    const { rerender } = render(panel([jikkyo, captions]));
    await userEvent.click(screen.getByText('実況'));
    // ライブ視聴で、実況を切ったとき。
    rerender(panel([captions]));
    expect(screen.getByText('字幕の行')).toBeDefined();
  });

  test('中身が 1 つなら、切り替えを出さず見出しだけにする', () => {
    render(panel([captions]));
    expect(screen.queryByRole('radiogroup')).toBeNull();
    expect(screen.getByText('字幕')).toBeDefined();
  });
});
