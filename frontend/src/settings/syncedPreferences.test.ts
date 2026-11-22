import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import {
  applyServerPreferences,
  preferenceChanged,
  registerPreferenceUploader,
  resetSyncedPreferencesForTest,
  type PreferenceChange,
} from './syncedPreferences';

const keymap = 'kotatsu.player.keymap';
const cmAutoSkip = 'kotatsu.player.cmAutoSkip';
const volume = 'kotatsu.player.volume';

beforeEach(() => {
  vi.useFakeTimers();
  localStorage.clear();
  resetSyncedPreferencesForTest();
});

afterEach(() => {
  vi.useRealTimers();
});

// uploads は、送った変更を集める送り手を置き、集めたものを返す。done は呼ばずに持っておく。
function uploads() {
  const sent: PreferenceChange[][] = [];
  const done: (() => void)[] = [];
  registerPreferenceUploader((changes, finish) => {
    sent.push(changes);
    done.push(finish);
  });

  return { sent, done };
}

describe('端末をまたいで同期する設定 (#1862)', () => {
  test('変えた設定を、続けて変えた分をまとめて、最後の値で送る', () => {
    const { sent } = uploads();
    localStorage.setItem(keymap, '{"a":1}');
    preferenceChanged(keymap);
    localStorage.setItem(keymap, '{"a":2}');
    preferenceChanged(keymap);
    localStorage.removeItem(cmAutoSkip);
    preferenceChanged(cmAutoSkip);
    vi.runAllTimers();

    expect(sent).toEqual([
      [
        { key: keymap, value: '{"a":2}' },
        { key: cmAutoSkip, value: null },
      ],
    ]);
  });

  test('同期しない設定は送らない', () => {
    const { sent } = uploads();
    localStorage.setItem(volume, '0.5');
    preferenceChanged(volume);
    vi.runAllTimers();

    expect(sent).toEqual([]);
  });

  test('サーバーの値を写して知らせ、最初はサーバーにない端末の値を送って引き継ぐ', () => {
    const { sent } = uploads();
    localStorage.setItem(keymap, 'local');
    localStorage.setItem(cmAutoSkip, 'false');
    const events: (string | null)[] = [];
    const listener = (event: StorageEvent) => events.push(event.key);
    window.addEventListener('storage', listener);

    applyServerPreferences([{ key: cmAutoSkip, value: 'true' }], localStorage);
    vi.runAllTimers();
    window.removeEventListener('storage', listener);

    expect(localStorage.getItem(cmAutoSkip)).toBe('true');
    expect(events).toEqual([cmAutoSkip]);
    expect(sent).toEqual([[{ key: keymap, value: 'local' }]]);
  });

  test('最初に揃えた後にサーバーから消えた値は、この端末でも消す', () => {
    uploads();
    applyServerPreferences([{ key: keymap, value: 'server' }], localStorage);
    applyServerPreferences([], localStorage);

    expect(localStorage.getItem(keymap)).toBeNull();
  });

  test('引き継ぎは端末ごとに 1 回だけで、開き直しても、サーバーで消した値を送り直さない', () => {
    uploads();
    applyServerPreferences([], localStorage);
    vi.runAllTimers();
    // 開き直した (覚えた状態を捨てた) 後に、ほかの端末で消した値が、この端末に残っていたとする。
    resetSyncedPreferencesForTest();
    const { sent } = uploads();
    localStorage.setItem(keymap, 'stale');
    applyServerPreferences([], localStorage);
    vi.runAllTimers();

    expect(sent).toEqual([]);
    expect(localStorage.getItem(keymap)).toBeNull();
  });

  test('送っている途中の設定は、サーバーの古い値で戻さない', () => {
    const { done } = uploads();
    applyServerPreferences([{ key: keymap, value: 'old' }], localStorage);
    localStorage.setItem(keymap, 'new');
    preferenceChanged(keymap);
    // 送る前に読み直しても、送り終えるまでは、変えた値のまま。
    applyServerPreferences([{ key: keymap, value: 'old' }], localStorage);
    vi.runAllTimers();
    applyServerPreferences([{ key: keymap, value: 'old' }], localStorage);
    expect(localStorage.getItem(keymap)).toBe('new');

    for (const finish of done) {
      finish();
    }
    applyServerPreferences([{ key: keymap, value: 'new' }], localStorage);
    expect(localStorage.getItem(keymap)).toBe('new');
  });

  test('送っている途中に変えた設定は、返事が来てから送る', () => {
    const { sent, done } = uploads();
    localStorage.setItem(keymap, 'first');
    preferenceChanged(keymap);
    vi.runAllTimers();
    localStorage.setItem(keymap, 'second');
    preferenceChanged(keymap);
    vi.runAllTimers();
    // 先の返事が来るまでは、次を送らない。
    expect(sent).toEqual([[{ key: keymap, value: 'first' }]]);

    done[0]?.();
    vi.runAllTimers();
    expect(sent).toEqual([[{ key: keymap, value: 'first' }], [{ key: keymap, value: 'second' }]]);
  });
});
