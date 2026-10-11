import { render, renderHook, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import type { JikkyoComment } from './comments';
import { useRecordedJikkyo } from './useRecordedJikkyo';

// added は、偽の CommentCanvas に渡った過去ログ。canvas を作るたびに、別の配列にする。
const added: JikkyoComment[][] = [];

vi.mock('./CommentCanvas', async () => {
  const { useImperativeHandle } = await import('react');

  return {
    CommentCanvas: ({
      ref,
    }: {
      ref: (handle: { add: (comments: JikkyoComment[]) => void } | null) => void;
    }) => {
      useImperativeHandle(ref, () => {
        const mine: JikkyoComment[] = [];
        added.push(mine);

        return { add: (comments: JikkyoComment[]) => mine.push(...comments) };
      });

      return null;
    },
  };
});

const comments: JikkyoComment[] = [{ time: 1.5, content: 'わこつ', mail: '184' }];

afterEach(() => {
  added.length = 0;
  vi.unstubAllGlobals();
});

describe('useRecordedJikkyo', () => {
  test('取った過去ログを、作り直した canvas にも渡し、実況を入れ直しても取り直さない', async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ channel: { id: 'jk1' }, comments })),
    );
    vi.stubGlobal('fetch', fetchMock);
    const url = '/media/recordings/r/jikkyo.json';
    const { result, rerender } = renderHook(
      ({ src }: { src: string | null }) => useRecordedJikkyo(src, () => 0, true),
      {
        initialProps: { src: url as string | null },
      },
    );
    const first = render(<>{result.current.overlay}</>);
    await waitFor(() => expect(result.current.state).toEqual({ kind: 'loaded', count: 1 }));
    expect(added.at(-1)).toEqual(comments);

    // 画質を変えると、canvas を作り直す。
    first.unmount();
    render(<>{result.current.overlay}</>);
    expect(added.at(-1)).toEqual(comments);

    // 実況を切って入れ直す。
    rerender({ src: null });
    rerender({ src: url });
    await waitFor(() => expect(result.current.state).toEqual({ kind: 'loaded', count: 1 }));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  test('取れなければ、エラーにする', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('bad gateway', { status: 502 })),
    );
    const { result } = renderHook(() =>
      useRecordedJikkyo('/media/recordings/r/jikkyo.json', () => 0, true),
    );

    await waitFor(() => expect(result.current.state).toEqual({ kind: 'error' }));
  });
});
