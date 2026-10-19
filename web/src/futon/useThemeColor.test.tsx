import { localStorageColorSchemeManager, MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, test } from 'vitest';

import '../i18n';
import { ColorSchemeToggle } from './ColorSchemeToggle';
import { theme } from './theme';
import { useThemeColor } from './useThemeColor';

function App() {
  useThemeColor();

  return <ColorSchemeToggle />;
}

beforeEach(() => {
  const meta = document.createElement('meta');
  meta.name = 'theme-color';
  meta.content = '#ff7407';
  document.head.append(meta);
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  document.head.querySelector('meta[name="theme-color"]')?.remove();
  document.documentElement.removeAttribute('data-mantine-color-scheme');
});

describe('useThemeColor', () => {
  test('アプリ内でテーマを切り替えると、theme-color が背景に合わせて変わる', async () => {
    render(
      <MantineProvider
        theme={theme}
        defaultColorScheme="light"
        colorSchemeManager={localStorageColorSchemeManager({ key: 'test-color-scheme' })}
      >
        <App />
      </MantineProvider>,
    );
    const meta = document.head.querySelector('meta[name="theme-color"]');
    expect(meta?.getAttribute('content')).toBe('#ff7407');

    await userEvent.click(screen.getByRole('radio', { name: 'ダーク' }));
    expect(meta?.getAttribute('content')).toBe('#2a221c');

    await userEvent.click(screen.getByRole('radio', { name: 'ライト' }));
    expect(meta?.getAttribute('content')).toBe('#ff7407');
  });
});
