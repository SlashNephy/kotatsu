import { act, render, renderHook, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import type { JikkyoComment } from './comments';
import { insertByTime, useRecordedJikkyo } from './useRecordedJikkyo';

// added は、偽の CommentCanvas に渡った過去ログ。canvas を作るたびに、別の配列にする。
const added: JikkyoComment[][] = [];
// canvasClock は、偽の CommentCanvas に渡った時計。
const canvasClock = vi.hoisted(() => ({ current: (): number => Number.NaN }));

vi.mock('./CommentCanvas', async () => {
  const { useImperativeHandle } = await import('react');

  return {
    CommentCanvas: ({
      ref,
      clock,
    }: {
      ref: (handle: { add: (comments: JikkyoComment[]) => void } | null) => void;
      clock: () => number;
    }) => {
      canvasClock.current = clock;
      useImperativeHandle(ref, () => {
        const mine: JikkyoComment[] = [];
        added.push(mine);

        return { add: (comments: JikkyoComment[]) => mine.push(...comments) };
      });

      return null;
    },
  };
});

// sessions は、偽の JikkyoSession。リアルタイムのコメントを、外から送れる。
const sessions = vi.hoisted(
  () =>
    [] as {
      channelId: string;
      onComment: (comment: {
        content: string;
        mail: string;
        at: number;
        postedAt?: number;
      }) => void;
      closed: boolean;
    }[],
);

vi.mock('./session', () => ({
  JikkyoSession: class {
    private readonly entry;

    constructor(
      channelId: string,
      handlers: {
        onComment: (comment: {
          content: string;
          mail: string;
          at: number;
          postedAt?: number;
        }) => void;
      },
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
    const url = '/internal/media/recordings/r/jikkyo.json';
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
      useRecordedJikkyo('/internal/media/recordings/r/jikkyo.json', () => 0, true),
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
      useRecordedJikkyo('/internal/media/recordings/r/jikkyo.json', () => 0, true, live),
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

  test('書き込まれた時刻が分かるコメントは、届いた時刻でなく、書き込まれた時刻の位置に流す (#1515)', async () => {
    const recordedFrom = '2026-10-16T10:00:00Z';
    const origin = '2026-10-16T10:00:02Z';
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ channel: { id: 'jk1' }, origin, comments }))),
    );
    const live = { channelId: 'jk1', recordedFrom, gaps: [] };
    const { result } = renderHook(() =>
      useRecordedJikkyo('/internal/media/recordings/r/jikkyo.json', () => 0, true, live),
    );
    render(<>{result.current.overlay}</>);
    await waitFor(() => expect(result.current.state).toEqual({ kind: 'loaded', count: 1 }));

    // 基準の 60 秒後に書き込まれ、2.5 秒遅れて届いたコメントは、60 秒の位置に流す。
    const postedAt = Date.parse(origin) + 60_000;
    const at = postedAt + 2_500 - performance.timeOrigin;
    act(() => sessions[0]!.onComment({ content: 'きたー', mail: '', at, postedAt }));

    expect(added.at(-1)?.at(-1)).toEqual({ time: 60, content: 'きたー', mail: '' });
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
          '/internal/media/recordings/r/jikkyo.json',
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

  test('過去ログが届く前に届いたリアルタイムのコメントも、過去ログといっしょに残す', async () => {
    let resolveFetch: (response: Response) => void = () => {};
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise<Response>((resolve) => (resolveFetch = resolve))),
    );
    const origin = '2026-10-16T10:00:02Z';
    const live = { channelId: 'jk1', recordedFrom: '2026-10-16T10:00:00Z', gaps: [] };
    const { result } = renderHook(() =>
      useRecordedJikkyo('/internal/media/recordings/r/jikkyo.json', () => 0, true, live),
    );
    act(() =>
      sessions[0]!.onComment({
        content: '先に届いた',
        mail: '',
        at: Date.parse(origin) + 90_000 - performance.timeOrigin,
      }),
    );
    resolveFetch(new Response(JSON.stringify({ channel: { id: 'jk1' }, origin, comments })));

    await waitFor(() => expect(result.current.state).toEqual({ kind: 'loaded', count: 1 }));
    expect(result.current.comments?.map((comment) => comment.content)).toEqual([
      'わこつ',
      '先に届いた',
    ]);
  });
});

describe('insertByTime', () => {
  test.each([
    { name: '末尾より後なら、末尾に足す', time: 5, want: ['a', 'b', 'x'] },
    { name: '途中の時刻なら、順を崩さずに挟む', time: 1.5, want: ['a', 'x', 'b'] },
    { name: '同じ時刻なら、後ろに足す', time: 2, want: ['a', 'b', 'x'] },
  ])('$name', ({ time, want }) => {
    const list = [
      { time: 1, content: 'a', mail: '' },
      { time: 2, content: 'b', mail: '' },
    ];

    expect(insertByTime(list, { time, content: 'x', mail: '' }).map((c) => c.content)).toEqual(
      want,
    );
  });
});

describe('useRecordedJikkyo のコメントの位置', () => {
  afterEach(() => {
    window.localStorage.clear();
  });

  test.each([
    { name: 'ずらさなければ、再生位置のまま', stored: null, offset: 0, want: 10 },
    { name: '早めると、時計を進めて描く', stored: '-2000', offset: -2, want: 12 },
    { name: '遅らせると、時計を戻して描く', stored: '1500', offset: 1.5, want: 8.5 },
  ])('$name', ({ stored, offset, want }) => {
    if (stored !== null) {
      window.localStorage.setItem('kotatsu.player.commentOffsetMs', stored);
    }
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise<Response>(() => {})),
    );
    const { result } = renderHook(() => useRecordedJikkyo('/jikkyo.json', () => 10, true));
    render(<>{result.current.overlay}</>);

    expect(result.current.offset).toBe(offset);
    expect(canvasClock.current()).toBe(want);
  });
});
