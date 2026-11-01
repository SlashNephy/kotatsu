import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { Captions } from 'lucide-react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { CommandButton } from './PlayerControls';

afterEach(() => {
  cleanup();
});

describe('CommandButton', () => {
  test('押すと、登録簿の操作を呼ぶ', () => {
    const toggleCaptions = vi.fn();
    render(
      <MantineProvider>
        <CommandButton
          commands={{ toggleCaptions }}
          command="toggleCaptions"
          label="字幕"
          icon={Captions}
        />
      </MantineProvider>,
    );

    fireEvent.click(screen.getByRole('button', { name: '字幕' }));
    expect(toggleCaptions).toHaveBeenCalledOnce();
  });

  test('今使えない操作のボタンは、出さない', () => {
    // 実況のない局のように、その画面にない操作。
    render(
      <MantineProvider>
        <CommandButton
          commands={{ toggleCaptions: () => {} }}
          command="toggleJikkyo"
          label="実況"
          icon={Captions}
        />
      </MantineProvider>,
    );

    expect(screen.queryByRole('button')).toBeNull();
  });
});
