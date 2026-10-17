import { describe, expect, test } from 'vitest';

import {
  loadDataBroadcastZipcode,
  normalizeZipcode,
  saveDataBroadcastZipcode,
  writeZipcodeNVRAM,
  zipcodeNVRAMKey,
} from './dataBroadcastZipcode';

function memoryStorage(): Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> & {
  items: Map<string, string>;
} {
  const items = new Map<string, string>();

  return {
    items,
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => items.set(key, value),
    removeItem: (key) => items.delete(key),
  };
}

describe('normalizeZipcode', () => {
  test.each([
    { name: '7 桁の数字', input: '1000001', want: '1000001' },
    { name: 'ハイフン付き', input: '100-0001', want: '1000001' },
    { name: '全角の数字とハイフン', input: '１００－０００１', want: '1000001' },
    { name: '前後の空白', input: ' 1000001 ', want: '1000001' },
    { name: '空', input: '', want: '' },
    { name: '6 桁', input: '100000', want: null },
    { name: '8 桁', input: '10000011', want: null },
    { name: '数字でない', input: '100-000a', want: null },
  ])('$name', ({ input, want }) => {
    expect(normalizeZipcode(input)).toBe(want);
  });
});

describe('郵便番号の保存', () => {
  test('覚えた郵便番号を読み、空にすると忘れる', () => {
    const storage = memoryStorage();
    expect(loadDataBroadcastZipcode(storage)).toBe('');
    saveDataBroadcastZipcode(storage, '1000001');
    expect(loadDataBroadcastZipcode(storage)).toBe('1000001');
    saveDataBroadcastZipcode(storage, '');
    expect(loadDataBroadcastZipcode(storage)).toBe('');
  });

  test('壊れた値は、既定の空として読む', () => {
    const storage = memoryStorage();
    storage.setItem('kotatsu.player.dataBroadcastZipcode', 'abc');
    expect(loadDataBroadcastZipcode(storage)).toBe('');
  });

  test('web-bml の NVRAM には base64 で書き、空なら消す', () => {
    const storage = memoryStorage();
    writeZipcodeNVRAM(storage, '1000001');
    expect(atob(storage.items.get(zipcodeNVRAMKey) ?? '')).toBe('1000001');
    writeZipcodeNVRAM(storage, '');
    expect(storage.items.has(zipcodeNVRAMKey)).toBe(false);
  });
});
