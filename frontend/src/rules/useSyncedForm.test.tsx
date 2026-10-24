import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { useSyncedForm } from './useSyncedForm';

type Form = { name: string; minutes: string };

const same = (a: Form, b: Form) => a.name === b.name && a.minutes === b.minutes;

function setup(initial: Form) {
  return renderHook(({ initial }: { initial: Form }) => useSyncedForm(initial, same), {
    initialProps: { initial },
  });
}

afterEach(() => {
  cleanup();
});

describe('useSyncedForm', () => {
  test('触っていない間は、新しい値が届いたら入れ直す', () => {
    // ストアにある古い値で描いた後に、サーバーから新しい値が届く (#533)。
    const { result, rerender } = setup({ name: 'ルール', minutes: '' });

    rerender({ initial: { name: 'ルール', minutes: '15' } });

    expect(result.current.form).toEqual({ name: 'ルール', minutes: '15' });
    expect(result.current.outdated).toBe(false);
  });

  test('触った後に違う値が届いたら、入力を残して知らせ、読み込み直せる', () => {
    const { result, rerender } = setup({ name: 'ルール', minutes: '' });
    act(() => result.current.set({ name: '変えた名前' }));

    rerender({ initial: { name: 'ルール', minutes: '15' } });

    expect(result.current.form).toEqual({ name: '変えた名前', minutes: '' });
    expect(result.current.outdated).toBe(true);

    act(() => result.current.reload());

    expect(result.current.form).toEqual({ name: 'ルール', minutes: '15' });
    expect(result.current.outdated).toBe(false);
  });

  test('保存して、入力と同じ値が届いたら、触っていない状態に戻る', () => {
    const { result, rerender } = setup({ name: 'ルール', minutes: '' });
    act(() => result.current.set({ minutes: '15' }));

    // 保存の結果で、ストアが入力と同じ値になる。
    rerender({ initial: { name: 'ルール', minutes: '15' } });
    expect(result.current.outdated).toBe(false);

    // その後に届いた新しい値は、入れ直す。
    rerender({ initial: { name: '新しい名前', minutes: '15' } });
    expect(result.current.form).toEqual({ name: '新しい名前', minutes: '15' });
    expect(result.current.outdated).toBe(false);
  });
});
