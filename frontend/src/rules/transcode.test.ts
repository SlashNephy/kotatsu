import type { TFunction } from 'i18next';
import { describe, expect, test } from 'vitest';

import { optionOfPreset, presetOfOption, transcodeLabel } from './transcode';

// t は、キーと値をそのまま並べて返す。
const t = ((key: string, values?: Record<string, string>) =>
  values === undefined
    ? key
    : `${key}:${values['codec']}:${values['height']}`) as unknown as TFunction;

describe('transcodeLabel', () => {
  test.each([
    { name: '空なら変換しない', preset: '', want: 'rules.transcodeOff' },
    { name: 'H.264', preset: '720p', want: 'rules.transcodePreset:H.264:720p' },
    { name: 'HEVC', preset: '1080p-hevc', want: 'rules.transcodePreset:HEVC:1080p' },
  ])('$name', ({ preset, want }) => {
    expect(transcodeLabel(t, preset)).toBe(want);
  });
});

describe('optionOfPreset と presetOfOption', () => {
  test.each(['', '720p', '1080p-hevc'])('%s は行って戻る', (preset) => {
    expect(presetOfOption(optionOfPreset(preset))).toBe(preset);
  });

  test('変換しないは、Select の値を空にしない', () => {
    expect(optionOfPreset('')).not.toBe('');
  });
});
