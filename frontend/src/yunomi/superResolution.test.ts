import { describe, expect, test } from 'vitest';

import {
  loadSuperResolution,
  loadSuperResolutionMethod,
  loadSuperResolutionStrength,
} from './superResolution';

describe('loadSuperResolution', () => {
  test.each([
    { name: '覚えていなければ拡大しない', stored: null, want: false },
    { name: '入れたことを覚えていれば拡大する', stored: 'true', want: true },
    { name: '切ったことを覚えていれば拡大しない', stored: 'false', want: false },
  ])('$name', ({ stored, want }) => {
    expect(loadSuperResolution({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ拡大しない', () => {
    expect(
      loadSuperResolution({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(false);
  });
});

describe('loadSuperResolutionStrength', () => {
  test.each([
    { name: '覚えていなければ標準の 3', stored: null, want: 3 },
    { name: '選んだ強さを覚えていればその強さ', stored: '1', want: 1 },
    { name: '知らない値なら標準の 3', stored: '9', want: 3 },
    { name: '数でなければ標準の 3', stored: 'strong', want: 3 },
  ])('$name', ({ stored, want }) => {
    expect(loadSuperResolutionStrength({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ標準の 3', () => {
    expect(
      loadSuperResolutionStrength({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(3);
  });
});

describe('loadSuperResolutionMethod', () => {
  test.each([
    { name: '覚えていなければ FSR', stored: null, want: 'fsr' },
    { name: '選んだ方式を覚えていればその方式', stored: 'anime4k', want: 'anime4k' },
    { name: '知らない値なら FSR', stored: 'rife', want: 'fsr' },
  ])('$name', ({ stored, want }) => {
    expect(loadSuperResolutionMethod({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ FSR', () => {
    expect(
      loadSuperResolutionMethod({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe('fsr');
  });
});
