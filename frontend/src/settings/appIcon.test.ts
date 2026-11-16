import { afterEach, describe, expect, test } from 'vitest';

import { applyFavicon, loadAppIcon, saveAppIcon } from './appIcon';

describe('loadAppIcon', () => {
  test.each([
    { name: '覚えていなければ既定の組', stored: null, want: 'default' },
    { name: '選んだ組を覚えていればその組', stored: 'chatora', want: 'chatora' },
    { name: '既定の組を覚えていれば既定の組', stored: 'default', want: 'default' },
    { name: '知らない組なら既定の組', stored: 'p9-unknown', want: 'default' },
    { name: 'ファイル名を覚えていても既定の組', stored: 'coat-kuro.svg', want: 'default' },
    { name: '空なら既定の組', stored: '', want: 'default' },
  ])('$name', ({ stored, want }) => {
    expect(loadAppIcon({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ既定の組', () => {
    expect(
      loadAppIcon({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe('default');
  });

  test('端末がなければ既定の組', () => {
    expect(loadAppIcon(null)).toBe('default');
  });
});

describe('saveAppIcon', () => {
  test('覚えた組を次に読める', () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => void values.set(key, value),
    };
    saveAppIcon(storage, 'kijitora');
    expect(loadAppIcon(storage)).toBe('kijitora');
  });

  test('書けなくても、同じタブの画面には知らせる', () => {
    let notified = 0;
    const onChange = () => {
      notified += 1;
    };
    window.addEventListener('kotatsu:app-icon', onChange);
    try {
      expect(() =>
        saveAppIcon(
          {
            setItem: () => {
              throw new Error('quota');
            },
          },
          'kuro',
        ),
      ).not.toThrow();
    } finally {
      window.removeEventListener('kotatsu:app-icon', onChange);
    }
    expect(notified).toBe(1);
  });
});

describe('applyFavicon', () => {
  afterEach(() => {
    document.head.innerHTML = '';
  });

  test('今の favicon の向き先を替える', () => {
    document.head.innerHTML = '<link rel="icon" href="/icons/favicon.svg" type="image/svg+xml" />';
    applyFavicon('/icons/coat-chatora.svg');
    const links = document.head.querySelectorAll('link[rel~="icon"]');
    expect(links).toHaveLength(1);
    expect(links[0]?.getAttribute('href')).toBe('/icons/coat-chatora.svg');
  });

  test('favicon がなければ足す', () => {
    applyFavicon('/icons/favicon.svg');
    const links = document.head.querySelectorAll('link[rel~="icon"]');
    expect(links).toHaveLength(1);
    expect(links[0]?.getAttribute('href')).toBe('/icons/favicon.svg');
  });

  test('apple-touch-icon は替えない', () => {
    document.head.innerHTML =
      '<link rel="apple-touch-icon" href="data:image/png;base64,AA==" /><link rel="icon" href="/icons/favicon.svg" />';
    applyFavicon('/icons/coat-kijitora.svg');
    expect(document.head.querySelector('link[rel="apple-touch-icon"]')?.getAttribute('href')).toBe(
      'data:image/png;base64,AA==',
    );
  });
});
