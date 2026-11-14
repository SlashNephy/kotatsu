import { renderHook } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { useDocumentTitle } from './useDocumentTitle';

describe('useDocumentTitle', () => {
  afterEach(() => {
    document.title = 'Kotatsu';
  });

  test('番組名とサービス名を、囲み文字を外して並べ、離れたら元に戻す', () => {
    document.title = 'Kotatsu';
    const { rerender, unmount } = renderHook(({ parts }) => useDocumentTitle(parts), {
      initialProps: { parts: ['🈑ニュース7🈞', 'NHK総合1・東京'] as (string | null)[] },
    });
    expect(document.title).toBe('ニュース7 - NHK総合1・東京 | Kotatsu');

    // 番組が変わったら変える。番組がなければ、サービス名だけにする。
    rerender({ parts: [null, 'NHK総合1・東京'] });
    expect(document.title).toBe('NHK総合1・東京 | Kotatsu');

    // データが届くまでのように、すべて空なら Kotatsu だけにする。
    rerender({ parts: [null, ''] });
    expect(document.title).toBe('Kotatsu');

    unmount();
    expect(document.title).toBe('Kotatsu');
  });

  test('後から開いた画面がタイトルを変えていれば、離れても元に戻さない (#1731)', () => {
    document.title = 'Kotatsu';
    const watch = renderHook(() => useDocumentTitle(['ニュース7']));
    expect(document.title).toBe('ニュース7 | Kotatsu');
    const guide = renderHook(() => useDocumentTitle(['番組表']));
    expect(document.title).toBe('番組表 | Kotatsu');

    // 視聴の画面を閉じても、番組表のタイトルのままにする。
    watch.unmount();
    expect(document.title).toBe('番組表 | Kotatsu');
    // 閉じた視聴の画面のタイトルには戻さない。
    guide.unmount();
    expect(document.title).toBe('Kotatsu');
  });

  test('小さなプレイヤーに切り替わる前に開いた画面を離れても、番組名に戻さない (#1775)', () => {
    const watch = renderHook(({ enabled }) => useDocumentTitle(['ニュース7'], enabled), {
      initialProps: { enabled: true },
    });
    expect(document.title).toBe('ニュース7 | Kotatsu');

    // 録画の一覧へ移ると、一覧が先にタイトルを付け、その後で視聴の画面が小さなプレイヤーになる。
    const recordings = renderHook(() => useDocumentTitle(['録画']));
    watch.rerender({ enabled: false });
    expect(document.title).toBe('録画 | Kotatsu');

    // タイトルを付けないホームへ戻る。
    recordings.unmount();
    expect(document.title).toBe('Kotatsu');
    watch.unmount();
  });

  test('enabled が false の間は、タイトルを変えない (#1731)', () => {
    const guide = renderHook(() => useDocumentTitle(['番組表']));
    const { rerender, unmount } = renderHook(
      ({ parts, enabled }) => useDocumentTitle(parts, enabled),
      {
        initialProps: { parts: ['ニュース7'] as (string | null)[], enabled: false },
      },
    );
    expect(document.title).toBe('番組表 | Kotatsu');

    // 小さなプレイヤーの間に番組が変わっても、変えない。
    rerender({ parts: ['天気予報'], enabled: false });
    expect(document.title).toBe('番組表 | Kotatsu');

    // 視聴の画面に戻ったら、今の番組にする。
    rerender({ parts: ['天気予報'], enabled: true });
    expect(document.title).toBe('天気予報 | Kotatsu');
    unmount();
    expect(document.title).toBe('番組表 | Kotatsu');
    guide.unmount();
  });
});
