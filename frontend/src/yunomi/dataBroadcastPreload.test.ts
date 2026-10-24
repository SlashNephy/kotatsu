import { describe, expect, test } from 'vitest';

import { loadDataBroadcastPreload, saveDataBroadcastPreload } from './dataBroadcastPreload';

function memoryStorage() {
  const items = new Map<string, string>();

  return {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => {
      items.set(key, value);
    },
  };
}

describe('dataBroadcastPreload', () => {
  test('覚えた設定がなければ、プリロードしない', () => {
    expect(loadDataBroadcastPreload(memoryStorage())).toBe(false);
    expect(loadDataBroadcastPreload(null)).toBe(false);
  });

  test.each([true, false])('%s を覚えたら、次に読める', (preload) => {
    const storage = memoryStorage();
    saveDataBroadcastPreload(storage, preload);

    expect(loadDataBroadcastPreload(storage)).toBe(preload);
  });

  test('読めなければ、プリロードしない', () => {
    expect(
      loadDataBroadcastPreload({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(false);
  });
});
