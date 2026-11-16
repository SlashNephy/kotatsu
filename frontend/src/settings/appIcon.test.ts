import { describe, expect, test } from 'vitest';

import { loadAppIcon, saveAppIcon } from './appIcon';

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
