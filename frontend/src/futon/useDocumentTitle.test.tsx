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
    expect(document.title).toBe('ニュース7 - NHK総合1・東京 - Kotatsu');

    // 番組が変わったら変える。番組がなければ、サービス名だけにする。
    rerender({ parts: [null, 'NHK総合1・東京'] });
    expect(document.title).toBe('NHK総合1・東京 - Kotatsu');

    unmount();
    expect(document.title).toBe('Kotatsu');
  });
});
