import { localStorageColorSchemeManager, MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, test } from 'vitest';

import '../i18n';
import { ColorSchemeToggle } from './ColorSchemeToggle';
import { theme } from './theme';

afterEach(() => {
  cleanup();
  localStorage.clear();
  // Mantine は、選んだテーマを <html> の属性に書く。cleanup では消えないので、テストごとに消す。
  document.documentElement.removeAttribute('data-mantine-color-scheme');
});

describe('ColorSchemeToggle', () => {
  test.each([
    { choice: 'ダーク', want: 'dark', stored: 'dark' },
    { choice: 'ライト', want: 'light', stored: 'light' },
  ])(
    '$choice を選ぶと、$want のテーマになり、選択が保存される',
    async ({ choice, want, stored }) => {
      render(
        <MantineProvider
          theme={theme}
          defaultColorScheme="auto"
          colorSchemeManager={localStorageColorSchemeManager({ key: 'test-color-scheme' })}
        >
          <ColorSchemeToggle />
        </MantineProvider>,
      );

      await userEvent.click(screen.getByText(choice));

      expect(document.documentElement.dataset.mantineColorScheme).toBe(want);
      expect(localStorage.getItem('test-color-scheme')).toBe(stored);
    },
  );
});
