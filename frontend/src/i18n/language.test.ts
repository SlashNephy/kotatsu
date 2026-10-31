import { describe, expect, it } from 'vitest';

import { detectLanguage } from './language';

describe('detectLanguage', () => {
  it.each([
    { name: '日本語を先に選んでいる', preferred: ['ja-JP', 'en-US'], want: 'ja' },
    { name: '英語を先に選んでいる', preferred: ['en-GB', 'ja'], want: 'en' },
    { name: '対応しない言語は飛ばす', preferred: ['fr-FR', 'ja'], want: 'ja' },
    { name: '対応する言語がなければ英語', preferred: ['fr-FR', 'de'], want: 'en' },
    { name: '設定がなければ英語', preferred: [], want: 'en' },
    { name: '大文字の名前', preferred: ['JA'], want: 'ja' },
    { name: '下線で区切った名前', preferred: ['en_GB', 'ja'], want: 'en' },
    { name: '言語が分からない', preferred: [undefined, 'ja'], want: 'ja' },
  ])('$name', ({ preferred, want }) => {
    expect(detectLanguage(preferred)).toBe(want);
  });
});
