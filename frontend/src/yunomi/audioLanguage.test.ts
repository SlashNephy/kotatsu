import { describe, expect, test } from 'vitest';

import { kyusuAudioChoices } from './audio';
import { loadAudioLanguage, preferredSlot, type AudioLanguage } from './audioLanguage';

describe('preferredSlot', () => {
  const stereo = (language: string) => ({ componentType: 3, languages: [language] });

  test.each([
    {
      name: '優先する言語の音声が第 2 音声なら、第 2 音声',
      audios: [stereo('jpn'), stereo('eng')],
      language: 'eng',
      want: 2,
    },
    {
      name: 'デュアルモノの副音声の言語でも選ぶ',
      audios: [{ componentType: 2, languages: ['jpn', 'eng'] }],
      language: 'eng',
      want: 2,
    },
    {
      name: '優先する言語が主音声なら、第 1 音声',
      audios: [stereo('jpn'), stereo('eng')],
      language: 'jpn',
      want: 1,
    },
    {
      name: '優先する言語の音声がなければ null',
      audios: [stereo('jpn'), stereo('eng')],
      language: 'kor',
      want: null,
    },
    {
      name: '中国語は、2 つのコードのどちらでも選ぶ',
      audios: [stereo('jpn'), stereo('chi')],
      language: 'zho',
      want: 2,
    },
    {
      name: '音声が 1 つしかなければ null',
      audios: [stereo('eng')],
      language: 'eng',
      want: null,
    },
    {
      name: '優先する言語を決めていなければ null',
      audios: [stereo('jpn'), stereo('eng')],
      language: null,
      want: null,
    },
  ])('$name', ({ audios, language, want }) => {
    expect(preferredSlot(kyusuAudioChoices(audios), language as AudioLanguage | null)).toBe(want);
  });
});

describe('loadAudioLanguage', () => {
  test.each([
    { value: 'eng', want: 'eng' },
    { value: null, want: null },
    { value: 'xxx', want: null },
  ])('$value', ({ value, want }) => {
    expect(loadAudioLanguage({ getItem: () => value })).toBe(want);
  });
});
