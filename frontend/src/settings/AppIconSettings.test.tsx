import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test } from 'vitest';

import '../i18n';
import { AppIconSettings } from './AppIconSettings';
import { appIconUrls, useAppFavicon, useAppIcon } from './appIcon';

// Header は、ヘッダー (RootLayout) と同じく、選んだアイコンを favicon とロゴに当てる。
function Header() {
  useAppFavicon();
  const [id] = useAppIcon();

  return <img data-testid="logo" src={appIconUrls(id).app} alt="" />;
}

function renderSettings() {
  render(
    <MantineProvider>
      <Header />
      <AppIconSettings />
    </MantineProvider>,
  );
}

// checked は、名前が name の選択肢を選んでいるか。
const checked = (name: string) =>
  screen.getByRole('radio', { name }).getAttribute('aria-checked') === 'true';
const logo = () => screen.getByTestId('logo').getAttribute('src');
const favicon = () => document.head.querySelector('link[rel~="icon"]')?.getAttribute('href');

describe('AppIconSettings', () => {
  beforeEach(() => {
    localStorage.clear();
    document.head.innerHTML = '<link rel="icon" href="/icons/favicon.svg" type="image/svg+xml" />';
  });

  afterEach(() => {
    cleanup();
    localStorage.clear();
    document.head.innerHTML = '';
  });

  test('選んでいなければ既定の組を選んだ状態で出す', () => {
    renderSettings();

    expect(checked('三毛猫とみかん (既定)')).toBe(true);
    expect(favicon()).toBe('/icons/favicon.svg');
    expect(logo()).toBe('/icons/icon.svg');
  });

  test('選ぶと、すぐ favicon とロゴを替え、端末に覚える', () => {
    renderSettings();

    fireEvent.click(screen.getByRole('radio', { name: '茶トラと夕焼け' }));

    expect(checked('茶トラと夕焼け')).toBe(true);
    expect(checked('三毛猫とみかん (既定)')).toBe(false);
    expect(favicon()).toBe('/icons/p4-chatora-sunset-favicon.svg');
    expect(logo()).toBe('/icons/p4-chatora-sunset-app.svg');
    expect(localStorage.getItem('kotatsu.appIcon')).toBe('chatora-sunset');
  });

  test('開き直すと、覚えた組を出す', () => {
    localStorage.setItem('kotatsu.appIcon', 'mike-night');
    renderSettings();

    expect(checked('三毛猫と夜の部屋')).toBe(true);
    expect(favicon()).toBe('/icons/p1-mike-night-favicon.svg');
    expect(logo()).toBe('/icons/p1-mike-night-app.svg');
  });

  test('知らない組を覚えていれば、既定の組を出す', () => {
    localStorage.setItem('kotatsu.appIcon', 'removed-icon');
    renderSettings();

    expect(checked('三毛猫とみかん (既定)')).toBe(true);
    expect(favicon()).toBe('/icons/favicon.svg');
  });

  test('ほかのタブで選んだ組にも追う', () => {
    renderSettings();

    localStorage.setItem('kotatsu.appIcon', 'mike-sunset');
    fireEvent(window, new StorageEvent('storage', { key: 'kotatsu.appIcon' }));

    expect(checked('三毛猫と夕焼け')).toBe(true);
    expect(favicon()).toBe('/icons/p2-mike-sunset-favicon.svg');
  });
});
