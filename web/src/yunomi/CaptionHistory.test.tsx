import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { CaptionHistory } from './CaptionHistory';

afterEach(() => {
  cleanup();
});

describe('CaptionHistory', () => {
  test('録画では、行の名前に時刻と字幕の文を含め、押すとその字幕の位置を渡す', async () => {
    const onSelect = vi.fn();
    render(
      <MantineProvider env="test">
        <CaptionHistory
          entries={[{ key: 0, at: 61, text: 'こんにちは' }]}
          label={(entry) => `0:${entry.at}`}
          onSelect={onSelect}
        />
      </MantineProvider>,
    );

    await userEvent.click(screen.getByRole('button', { name: /0:61.*こんにちは/ }));

    expect(onSelect).toHaveBeenCalledWith({ key: 0, at: 61, text: 'こんにちは' });
  });

  test('ライブでは、行を押せない', () => {
    render(
      <MantineProvider env="test">
        <CaptionHistory
          entries={[{ key: 0, at: 0, text: 'こんにちは' }]}
          label={() => '21:00:00'}
        />
      </MantineProvider>,
    );

    expect(screen.queryByRole('button', { name: /こんにちは/ })).toBeNull();
  });
});
