import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';

import { CommentCanvas, type CommentCanvasHandle } from './CommentCanvas';
import type { JikkyoComment } from './comments';
import { recordingPosition, type Gap } from '../yunomi/position';
import { NXJikkyoSession } from './session';

type RecordedComments = {
  channel: { id: string } | null;
  // origin は、コメントの時刻の基準 (最初のセグメントを書き始めた時刻)。チャンネルがなければ null。
  origin: string | null;
  comments: JikkyoComment[];
};

export type RecordedJikkyoLive = {
  channelId: string;
  // recordedFrom と gaps は、放送時刻を録画の中の位置に直すのに使う。過去ログの時刻の基準 (origin) が分かれば、そちらを使う。
  recordedFrom: string;
  gaps: readonly Gap[];
};

export type RecordedJikkyoState =
  | { kind: 'loading' }
  | { kind: 'loaded'; count: number }
  | { kind: 'error' };

// insertByTime は、流す時刻の順に並んだ comments に、comment を順を崩さずに足した配列を返す。
// リアルタイムのコメントは、過去ログの最後のコメントより前の位置に当たることがある。
export function insertByTime(
  comments: readonly JikkyoComment[],
  comment: JikkyoComment,
): JikkyoComment[] {
  let i = comments.length;
  while (i > 0 && comments[i - 1]!.time > comment.time) {
    i--;
  }

  return [...comments.slice(0, i), comment, ...comments.slice(i)];
}

// useRecordedJikkyo は、録画の再生で、録画の時間帯の実況の過去ログを流す (設計書 7.4)。
// 過去ログは、tenban が過去ログ API から取って中継する。url が null なら、何もしない。
// clock は、録画の始まりからの再生位置 (秒) を返す。過去ログの時刻も、録画の始まりからの秒。
// live は、録画中の録画を追っかけ再生しているときの、実況のチャンネルと、放送時刻を再生位置に直す関数。
// 過去ログには、取った後のコメントがないので、録画中の間は、リアルタイムのコメントも受け取り、録画の中の位置に流す (#667)。
export function useRecordedJikkyo(
  url: string | null,
  clock: () => number,
  drawing: boolean,
  live: RecordedJikkyoLive | null = null,
): {
  overlay: ReactNode;
  state: RecordedJikkyoState | null;
  // comments は、取った過去ログ (流す時刻の順)。取れていなければ null (#489)。
  comments: readonly JikkyoComment[] | null;
} {
  // cache は、取った過去ログ。画質を変えて描く canvas が作り直されても、実況を切って入れ直しても、取り直さずに渡す。
  const cache = useRef(new Map<string, JikkyoComment[]>());
  const canvas = useRef<CommentCanvasHandle | null>(null);
  const [state, setState] = useState<RecordedJikkyoState | null>(null);
  const [comments, setComments] = useState<readonly JikkyoComment[] | null>(null);

  // canvas ができるたびに、取ってある過去ログを渡す。
  const attach = useCallback(
    (handle: CommentCanvasHandle | null) => {
      canvas.current = handle;
      const comments = url === null ? undefined : cache.current.get(url);
      if (handle !== null && comments !== undefined) {
        handle.add(comments);
      }
    },
    [url],
  );

  useEffect(() => {
    if (url === null) {
      setState(null);
      setComments(null);

      return;
    }
    const cached = cache.current.get(url);
    if (cached !== undefined) {
      setState({ kind: 'loaded', count: cached.length });
      setComments(cached);

      return;
    }
    setComments(null);
    const controller = new AbortController();
    setState({ kind: 'loading' });
    void fetch(url, { signal: controller.signal, credentials: 'same-origin' })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`unexpected status ${response.status}`);
        }

        return (await response.json()) as RecordedComments;
      })
      .then((body) => {
        // 過去ログが届く前に、リアルタイムのコメントが届いていることがある。上書きせず、流す時刻の順に合わせる。
        const received = cache.current.get(url) ?? [];
        const merged = [...body.comments, ...received].sort((a, b) => a.time - b.time);
        cache.current.set(url, merged);
        if (body.origin !== null) {
          origin.current.set(url, body.origin);
        }
        canvas.current?.add(body.comments);
        setState({ kind: 'loaded', count: body.comments.length });
        setComments(merged);
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setState({ kind: 'error' });
        }
      });

    return () => controller.abort();
  }, [url]);

  // 録画中の間は、リアルタイムのコメントを受け取り、届いた時刻の放送に当たる位置に流す。見えていない間は、接続を閉じる。
  // origin は、過去ログの時刻の基準。リアルタイムのコメントも、同じ基準の位置に流す。
  const origin = useRef(new Map<string, string>());
  const liveRef = useRef(live);
  liveRef.current = live;
  const liveChannel = live?.channelId ?? null;
  useEffect(() => {
    if (url === null || liveChannel === null || !drawing) {
      return;
    }
    const session = new NXJikkyoSession(liveChannel, {
      onComment: (received) => {
        const current = liveRef.current;
        if (current === null) {
          return;
        }
        const base = origin.current.get(url) ?? current.recordedFrom;
        // received.at は performance.now の時計なので、壁時計に直す。
        const comment = {
          time: recordingPosition(base, current.gaps, performance.timeOrigin + received.at),
          content: received.content,
          mail: received.mail,
        };
        canvas.current?.add([comment]);
        // 描く canvas を作り直しても、一覧からも、受け取った分を読めるようにする。一覧は、流す時刻の順に並べる。
        const merged = insertByTime(cache.current.get(url) ?? [], comment);
        cache.current.set(url, merged);
        setComments(merged);
      },
      onStatus: () => {},
    });

    return () => session.close();
  }, [url, liveChannel, drawing]);

  return {
    overlay:
      url === null ? undefined : (
        <CommentCanvas key={url} ref={attach} clock={clock} drawing={drawing} />
      ),
    state,
    comments,
  };
}
