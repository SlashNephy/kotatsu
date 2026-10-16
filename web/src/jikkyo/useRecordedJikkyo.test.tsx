import { act, render, renderHook, waitFor } from '@testing-library/react';
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

// sessions は、偽の NXJikkyoSession。リアルタイムのコメントを、外から送れる。
const sessions = vi.hoisted(
  () =>
    [] as {
      channelId: string;
      onComment: (comment: { content: string; mail: string; at: number }) => void;
      closed: boolean;
    }[],
);

vi.mock('./session', () => ({
  NXJikkyoSession: class {
    private readonly entry;

    constructor(
      channelId: string,
      handlers: { onComment: (comment: { content: string; mail: string; at: number }) => void },
    ) {
      this.entry = { channelId, onComment: handlers.onComment, closed: false };
      sessions.push(this.entry);
    }

    close() {
      this.entry.closed = true;
    }
  },
}));

const comments: JikkyoComment[] = [{ time: 1.5, content: 'わこつ', mail: '184' }];

afterEach(() => {
  added.length = 0;
  sessions.length = 0;
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

  test('録画中なら、リアルタイムのコメントも受け取り、過去ログと同じ基準の位置に流す', async () => {
    // 過去ログの基準 (最初のセグメントを書き始めた時刻) は、録画を始めた時刻の 2 秒後。
    const recordedFrom = '2026-10-16T10:00:00Z';
    const origin = '2026-10-16T10:00:02Z';
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ channel: { id: 'jk1' }, origin, comments }))),
    );
    const live = { channelId: 'jk1', recordedFrom, gaps: [] };
    const { result } = renderHook(() =>
      useRecordedJikkyo('/media/recordings/r/jikkyo.json', () => 0, true, live),
    );
    render(<>{result.current.overlay}</>);
    await waitFor(() => expect(result.current.state).toEqual({ kind: 'loaded', count: 1 }));
    expect(sessions.map((session) => session.channelId)).toEqual(['jk1']);

    // 基準の 60 秒後に届いたコメントは、60 秒の位置に流す。at は performance.now の時計。
    const at = Date.parse(origin) + 60_000 - performance.timeOrigin;
    act(() => sessions[0]!.onComment({ content: 'きたー', mail: '', at }));

    expect(added.at(-1)?.at(-1)).toEqual({ time: 60, content: 'きたー', mail: '' });
    expect(result.current.comments?.map((comment) => comment.content)).toEqual([
      'わこつ',
      'きたー',
    ]);
  });

  test('録画を終えたら、リアルタイムのコメントを受け取るのをやめる', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () => new Response(JSON.stringify({ channel: null, origin: null, comments: [] })),
      ),
    );
    const live = { channelId: 'jk1', recordedFrom: '2026-10-16T10:00:00Z', gaps: [] };
    const { rerender } = renderHook(
      ({ recording }: { recording: boolean }) =>
        useRecordedJikkyo(
          '/media/recordings/r/jikkyo.json',
          () => 0,
          true,
          recording ? live : null,
        ),
      { initialProps: { recording: true } },
    );
    expect(sessions).toHaveLength(1);

    rerender({ recording: false });

    expect(sessions[0]!.closed).toBe(true);
  });
});
