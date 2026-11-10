import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import { CommentCanvas, type CommentCanvasHandle } from './CommentCanvas';
import {
  compileMute,
  defaultCommentSettings,
  type CommentSettings,
  type JikkyoComment,
} from './comments';
import { useCommentOffset } from './commentOffset';
import { alignNicolog, type CmSection } from './alignNicolog';
import { mixBroadcast } from './mixBroadcasts';
import { findPartMarkers } from './partMarkers';
import { recordingPosition, type Gap } from '../yunomi/position';
import { JikkyoSession } from './session';

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
// settings は、コメントの表示とミュートの設定 (#488)。ミュートしたコメントは、流さず、一覧にも出さない。
export function useRecordedJikkyo(
  url: string | null,
  clock: () => number,
  drawing: boolean,
  live: RecordedJikkyoLive | null = null,
  settings: CommentSettings = defaultCommentSettings,
  // othersUrl は、同じ話の別の放送の過去ログの URL (#1361)。設定で有効にしていて、録画を終えているときだけ読む。
  othersUrl: string | null = null,
  // nicolog は、上映会のコメント (NICOLOG) の URL と、録画の CM の区間 (#1363)。ソースをオプトインしていれば渡す。
  nicolog: { url: string; cmSections: readonly CmSection[] } | null = null,
): {
  overlay: ReactNode;
  state: RecordedJikkyoState | null;
  // comments は、取った過去ログ (流す時刻の順)。別の放送の過去ログを混ぜていれば、それも含む。取れていなければ null (#489)。
  comments: readonly JikkyoComment[] | null;
  // ownComments は、録画自身の過去ログ。番組の区切りを見つけるのに使う。
  ownComments: readonly JikkyoComment[] | null;
  // offset は、コメントを流す位置をずらす秒数 (#1357)。正の値で遅らせる。一覧も、この分をずらして出す。
  offset: number;
} {
  // cache は、取った過去ログ。画質を変えて描く canvas が作り直されても、実況を切って入れ直しても、取り直さずに渡す。
  const cache = useRef(new Map<string, JikkyoComment[]>());
  const canvas = useRef<CommentCanvasHandle | null>(null);
  const [state, setState] = useState<RecordedJikkyoState | null>(null);
  const [comments, setComments] = useState<readonly JikkyoComment[] | null>(null);
  const [offsetMs] = useCommentOffset();
  // othersCache は、取った別の放送の過去ログ。切り替えても取り直さない。
  const othersCache = useRef(new Map<string, JikkyoComment[][]>());
  const [others, setOthers] = useState<{ url: string; logs: JikkyoComment[][] } | null>(null);
  const othersTarget = settings.mixOtherBroadcasts && live === null ? othersUrl : null;
  useEffect(() => {
    if (othersTarget === null) {
      return;
    }
    const cached = othersCache.current.get(othersTarget);
    if (cached !== undefined) {
      setOthers({ url: othersTarget, logs: cached });

      return;
    }
    const controller = new AbortController();
    void fetch(othersTarget, { signal: controller.signal, credentials: 'same-origin' })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`unexpected status ${response.status}`);
        }

        return (await response.json()) as { broadcasts: { comments: JikkyoComment[] }[] };
      })
      .then((body) => {
        const logs = body.broadcasts.map((broadcast) => broadcast.comments);
        othersCache.current.set(othersTarget, logs);
        setOthers({ url: othersTarget, logs });
      })
      .catch(() => {
        // 取れなければ、録画自身の過去ログだけを流す。
      });

    return () => controller.abort();
  }, [othersTarget]);
  // mixed は、録画の位置に合わせた別の放送のコメント。区切りが足りない放送は混ぜない。
  const mixed = useMemo(
    () =>
      comments === null || others === null || others.url !== othersTarget
        ? []
        : others.logs.flatMap((log) => mixBroadcast(comments, log)),
    [comments, others, othersTarget],
  );
  // nicologCache は、取った上映会のコメント。切り替えても取り直さない。
  const nicologCache = useRef(new Map<string, JikkyoComment[]>());
  const [nicologLog, setNicologLog] = useState<{ url: string; comments: JikkyoComment[] } | null>(
    null,
  );
  const nicologUrl = live === null && nicolog !== null ? nicolog.url : null;
  useEffect(() => {
    if (nicologUrl === null) {
      return;
    }
    const cached = nicologCache.current.get(nicologUrl);
    if (cached !== undefined) {
      setNicologLog({ url: nicologUrl, comments: cached });

      return;
    }
    const controller = new AbortController();
    void fetch(nicologUrl, { signal: controller.signal, credentials: 'same-origin' })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`unexpected status ${response.status}`);
        }

        return (await response.json()) as { comments: JikkyoComment[] };
      })
      .then((body) => {
        nicologCache.current.set(nicologUrl, body.comments);
        setNicologLog({ url: nicologUrl, comments: body.comments });
      })
      .catch(() => {
        // 取れなければ、ほかのコメントだけを流す。
      });

    return () => controller.abort();
  }, [nicologUrl]);
  // aligned は、録画の位置に合わせた上映会のコメント。録画自身の過去ログの `OP` で合わせるので、それがなければ混ぜない。
  const nicologSections = nicolog?.cmSections;
  const aligned = useMemo(
    () =>
      comments === null ||
      nicologLog === null ||
      nicologLog.url !== nicologUrl ||
      nicologSections === undefined
        ? []
        : alignNicolog(
            nicologLog.comments,
            findPartMarkers(nicologLog.comments),
            findPartMarkers(comments),
            nicologSections,
          ),
    [comments, nicologLog, nicologUrl, nicologSections],
  );
  const extra = useMemo(() => [...mixed, ...aligned], [mixed, aligned]);
  const mixedRef = useRef(extra);
  mixedRef.current = extra;
  const merged = useMemo(
    () =>
      comments === null || extra.length === 0
        ? comments
        : [...comments, ...extra].sort((a, b) => a.time - b.time),
    [comments, extra],
  );
  const offset = offsetMs / 1_000;

  // canvas ができるたびに、取ってある過去ログを渡す。
  const attach = useCallback(
    (handle: CommentCanvasHandle | null) => {
      canvas.current = handle;
      const comments = url === null ? undefined : cache.current.get(url);
      if (handle !== null && comments !== undefined) {
        handle.add([...comments, ...mixedRef.current]);
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
    const session = new JikkyoSession(liveChannel, {
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
          ...(received.userId === undefined ? {} : { userId: received.userId }),
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

  // 一覧には、ミュートしたコメントを出さない。
  // ミュートの一覧は、ほかの設定を変えただけでも新しい配列で届くので、中身で比べる。
  const muteKey = JSON.stringify([
    settings.mutedKeywords,
    settings.mutedUsers,
    settings.mutedCommands,
  ]);
  const listed = useMemo(() => {
    const [keywords, users, commands] = JSON.parse(muteKey) as [
      CommentSettings['mutedKeywords'],
      string[],
      string[],
    ];
    const muted = compileMute({
      ...defaultCommentSettings,
      mutedKeywords: keywords,
      mutedUsers: users,
      mutedCommands: commands,
    });

    return merged?.filter((comment) => !muted(comment)) ?? null;
  }, [merged, muteKey]);

  return {
    overlay:
      url === null ? undefined : (
        <CommentCanvas
          // 混ぜ始めたら描き直す。canvas を作り直すと、attach が混ぜたコメントも渡す。
          key={`${url}:${String(extra.length > 0)}`}
          ref={attach}
          clock={() => clock() - offset}
          drawing={drawing}
          settings={settings}
        />
      ),
    state,
    comments: listed,
    ownComments: comments,
    offset,
  };
}
