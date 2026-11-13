import { describe, expect, test } from 'vitest';

import { clearLiveResume, loadLiveResume, maxLiveResumes, saveLiveResume } from './liveResume';

// memoryStorage は、localStorage の代わりに、Map に読み書きする。
function memoryStorage(): Pick<Storage, 'getItem' | 'setItem'> {
  const items = new Map<string, string>();

  return {
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
  };
}

describe('liveResume', () => {
  test('サービスごとに、見ていた放送時刻を覚え、消せる', () => {
    const storage = memoryStorage();
    saveLiveResume(storage, 'a', 1_000, 5_000);
    saveLiveResume(storage, 'b', 2_000, 5_000);
    saveLiveResume(storage, 'a', 1_500, 6_000);

    expect(loadLiveResume(storage, 'a')).toBe(1_500);
    expect(loadLiveResume(storage, 'b')).toBe(2_000);
    expect(loadLiveResume(storage, 'c')).toBeNull();

    clearLiveResume(storage, 'a');
    expect(loadLiveResume(storage, 'a')).toBeNull();
    expect(loadLiveResume(storage, 'b')).toBe(2_000);
  });

  test('上限を超えたら、最後に覚えたのが古いサービスから忘れる', () => {
    const storage = memoryStorage();
    for (let i = 0; i <= maxLiveResumes; i++) {
      saveLiveResume(storage, `s${i}`, i, i);
    }

    expect(loadLiveResume(storage, 's0')).toBeNull();
    expect(loadLiveResume(storage, 's1')).toBe(1);
    expect(loadLiveResume(storage, `s${maxLiveResumes}`)).toBe(maxLiveResumes);
  });

  test.each([
    { name: '読めない値', raw: 'not json' },
    { name: '形の違う値', raw: '{"a":"x"}' },
    { name: '配列の要素の形が違う値', raw: '[{"service":"a"}]' },
  ])('$name が入っていても、覚えていないものとして扱う', ({ raw }) => {
    const storage = memoryStorage();
    storage.setItem('kotatsu.player.liveResume', raw);

    expect(loadLiveResume(storage, 'a')).toBeNull();
    // 壊れた値は、次に覚えるときに置き換える。
    saveLiveResume(storage, 'a', 1_000, 2_000);
    expect(loadLiveResume(storage, 'a')).toBe(1_000);
  });

  test('読み書きできない端末でも、投げずに続ける', () => {
    const broken: Pick<Storage, 'getItem' | 'setItem'> = {
      getItem: () => {
        throw new Error('SecurityError');
      },
      setItem: () => {
        throw new Error('QuotaExceededError');
      },
    };

    expect(() => saveLiveResume(broken, 'a', 1, 1)).not.toThrow();
    expect(loadLiveResume(broken, 'a')).toBeNull();
    expect(() => clearLiveResume(broken, 'a')).not.toThrow();
    expect(loadLiveResume(null, 'a')).toBeNull();
  });
});
