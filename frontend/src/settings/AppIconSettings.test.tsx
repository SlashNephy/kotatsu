import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { AppIconSettings } from './AppIconSettings';
import { appIconUrls, saveAppIcon, useAppFavicon, useAppIcon } from './appIcon';

// Header は、ヘッダー (RootLayout) と同じく、選んだアイコンを favicon とロゴに当てる。
function Header() {
  const [id] = useAppIcon();
  useAppFavicon(id);

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

    fireEvent.click(screen.getByRole('radio', { name: 'サバトラ' }));

    expect(checked('サバトラ')).toBe(true);
    expect(checked('三毛猫とみかん (既定)')).toBe(false);
    expect(favicon()).toBe('/icons/coat-sabatora.svg');
    expect(logo()).toBe('/icons/coat-sabatora.svg');
    expect(localStorage.getItem('kotatsu.appIcon')).toBe('sabatora');
  });

  test('開き直すと、覚えた組を出す', () => {
    localStorage.setItem('kotatsu.appIcon', 'kuro');
    renderSettings();

    expect(checked('黒猫')).toBe(true);
    expect(favicon()).toBe('/icons/coat-kuro.svg');
    expect(logo()).toBe('/icons/coat-kuro.svg');
  });

  test('知らない組を覚えていれば、既定の組を出す', () => {
    localStorage.setItem('kotatsu.appIcon', 'removed-icon');
    renderSettings();

    expect(checked('三毛猫とみかん (既定)')).toBe(true);
    expect(favicon()).toBe('/icons/favicon.svg');
  });

  test('端末に覚えられなくても、このタブでは選んだ組にする', () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });
    try {
      renderSettings();

      fireEvent.click(screen.getByRole('radio', { name: '茶トラ' }));

      expect(checked('茶トラ')).toBe(true);
      expect(favicon()).toBe('/icons/coat-chatora.svg');
      expect(logo()).toBe('/icons/coat-chatora.svg');
      expect(localStorage.getItem('kotatsu.appIcon')).toBeNull();
    } finally {
      setItem.mockRestore();
      // 覚えられた組に戻し、このタブだけの組をほかのテストに残さない。
      saveAppIcon(localStorage, 'default');
    }
  });

  test('ほかのタブで選んだ組にも追う', () => {
    renderSettings();

    localStorage.setItem('kotatsu.appIcon', 'kijitora');
    fireEvent(window, new StorageEvent('storage', { key: 'kotatsu.appIcon' }));

    expect(checked('キジトラ')).toBe(true);
    expect(favicon()).toBe('/icons/coat-kijitora.svg');
  });
});
