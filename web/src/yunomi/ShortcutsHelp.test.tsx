import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import '../i18n';
import { ShortcutsHelp } from './ShortcutsHelp';

afterEach(() => {
  cleanup();
});

describe('ShortcutsHelp', () => {
  test('渡した操作だけを、割り当てたキーとともに並べる', () => {
    render(
      <MantineProvider>
        <ShortcutsHelp
          opened
          onClose={() => {}}
          actions={['togglePlay', 'seekBackward']}
          target={{ current: null }}
        />
      </MantineProvider>,
    );

    const rows = screen.getAllByRole('row').map((row) =>
      within(row)
        .getAllByRole('cell')
        .map((cell) => cell.textContent),
    );
    expect(rows).toEqual([
      ['再生と一時停止', 'Spacek'],
      ['10 秒戻る', '←j'],
    ]);
  });

  test('ライブ視聴では、データ放送を出している間に使えるキーも並べる', () => {
    render(
      <MantineProvider>
        <ShortcutsHelp
          opened
          onClose={() => {}}
          actions={['togglePlay']}
          target={{ current: null }}
          dataBroadcast
        />
      </MantineProvider>,
    );

    const rows = screen.getAllByRole('row').map((row) =>
      within(row)
        .getAllByRole('cell')
        .map((cell) => cell.textContent),
    );
    expect(rows).toEqual([
      ['再生と一時停止', 'Spacek'],
      ['矢印', 'Alt↑↓←→'],
      ['決定', 'Enter'],
      ['戻る', 'Backspace'],
      ['青・赤・緑・黄', 'AltBRGY'],
      ['数字', '0〜9'],
    ]);
  });
});
