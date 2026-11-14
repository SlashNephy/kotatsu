import { useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from 'react';

import { appendComment, liveForce, type ListedComment } from './commentList';
import { CommentCanvas, type CommentCanvasHandle } from './CommentCanvas';
import { compileMute, defaultCommentSettings, type CommentSettings } from './comments';
import { syncedCommentWait, type BroadcastNow } from './liveSync';
import { JikkyoSession, type SessionStatus } from './session';

// liveRetainSeconds は、ライブで、設定を変えて描き直すときに足し直すコメントの秒数。流れている間のコメントが収まればよい。
const liveRetainSeconds = 30;

// maxDelay は、コメントを遅らせる上限 (秒)。読み込みが止まって先読みが大きくなっても、コメントを待たせすぎない。
const maxDelay = 10;

// playbackDelay は、映像が放送からどれだけ遅れて映っているか (秒) を返す。先に読んだ分だけ遅れている。
// コメントは放送の時刻で届くので、この分だけ遅らせて流すと、映像の場面と合う。
export function playbackDelay(video: HTMLVideoElement | null): number {
  if (video === null || video.buffered.length === 0) {
    return 0;
  }

  return Math.min(
    Math.max(video.buffered.end(video.buffered.length - 1) - video.currentTime, 0),
    maxDelay,
  );
}

// useLiveJikkyo は、ライブ視聴で実況のコメントを受け取って流し、投稿できるようにする (設計書 7.4)。
// channelId が null なら、何もしない。active が false の間は、接続を閉じる。
// settings は、コメントの表示とミュートの設定 (#488)。ミュートしたコメントは、流さず、一覧にも出さない。
// broadcast は、映しているコマの放送からの遅れを返す (#1515)。返せれば、コメントを書き込まれた時刻の場面に合わせて流し、
// 返せなければ (kyusu の経路や、放送の時計がまだ届かない間)、受け取った時刻に先読みの分を足して流す。
export function useLiveJikkyo(
  channelId: string | null,
  videoRef: RefObject<HTMLVideoElement | null>,
  active: boolean,
  settings: CommentSettings = defaultCommentSettings,
  broadcast: () => BroadcastNow | null = noBroadcast,
): {
  overlay: ReactNode;
  status: SessionStatus | null;
  // comments は、届いたコメントの一覧 (#489)。新しいものが末尾。
  comments: readonly ListedComment[];
  // force は、届いたコメントから数えた勢い (1 分あたりのコメントの数。#490)。つないだ直後と、つないでいない間は null。
  force: number | null;
  post: (text: string) => Promise<void>;
} {
  const canvas = useRef<CommentCanvasHandle | null>(null);
  // コメントを描く時計。画面を開いてからの秒。
  const origin = useRef(performance.now());
  const clock = () => (performance.now() - origin.current) / 1000;
  const sessionRef = useRef<JikkyoSession | null>(null);
  const [status, setStatus] = useState<SessionStatus | null>(null);
  const [comments, setComments] = useState<readonly ListedComment[]>([]);
  // arrivals は、直近に届いたコメントの時刻 (performance.now)。ミュートしたものも数える。勢いは、局の盛り上がりを表すからである。
  const arrivals = useRef<number[]>([]);
  const [force, setForce] = useState<number | null>(null);
  // ミュートは、届いたときの設定で決める。接続し直さないよう、ref で読む。
  const muted = useRef(compileMute(settings));
  muted.current = compileMute(settings);
  const broadcastRef = useRef(broadcast);
  broadcastRef.current = broadcast;

  useEffect(() => {
    if (channelId === null || !active) {
      return;
    }
    const session = new JikkyoSession(channelId, {
      onComment: (comment) => {
        arrivals.current.push(comment.at);
        const wait = commentWait(comment.postedAt, broadcastRef.current(), videoRef.current);
        const drawn = {
          time: (comment.at - origin.current) / 1000 + (wait ?? 0),
          content: comment.content,
          mail: comment.mail,
          ...(comment.userId === undefined ? {} : { userId: comment.userId }),
        };
        // 映している場面を大きく過ぎて届いたコメントは、場面と合わないので流さない。一覧には残す。
        if (wait !== null) {
          canvas.current?.add([drawn]);
        }
        if (muted.current(drawn)) {
          return;
        }
        // 一覧には、受け取った時刻を壁時計 (Unix ミリ秒) で残す。comment.at は performance.now の時計。
        setComments((list) =>
          appendComment(list, { ...comment, at: performance.timeOrigin + comment.at }),
        );
      },
      onStatus: setStatus,
    });
    sessionRef.current = session;
    // 勢いは、5 秒ごとに数え直す。コメントが届かなくなっても、下がっていくようにする。
    const since = performance.now();
    arrivals.current = [];
    const timer = window.setInterval(() => {
      const now = performance.now();
      arrivals.current = arrivals.current.filter((at) => at > now - 60_000);
      setForce(liveForce(arrivals.current, now, since));
    }, 5_000);

    return () => {
      session.close();
      sessionRef.current = null;
      window.clearInterval(timer);
      setStatus(null);
      setForce(null);
    };
  }, [channelId, active, videoRef]);

  // 局を変えたら、前の局のコメントは捨てる。見えなくなって接続を閉じている間は、読み返せるよう残す。
  useEffect(() => {
    setComments([]);
  }, [channelId]);

  const post = async (text: string) => {
    const session = sessionRef.current;
    if (session === null) {
      throw new Error('not connected');
    }
    await session.post(text);
    // 自分の投稿は、届くのを待たずに、今の場面に流す。
    canvas.current?.add([{ time: clock(), content: text, mail: '184' }]);
    setComments((list) => appendComment(list, { at: Date.now(), content: text, mail: '184' }));
  };

  // 届いた後にミュートしたユーザーやキーワードのコメントも、一覧から外す (#767)。
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
    const isMuted = compileMute({
      ...defaultCommentSettings,
      mutedKeywords: keywords,
      mutedUsers: users,
      mutedCommands: commands,
    });

    return comments.filter((comment) => !isMuted({ time: 0, ...comment }));
  }, [comments, muteKey]);

  return {
    overlay:
      channelId === null ? undefined : (
        <CommentCanvas
          key={channelId}
          ref={canvas}
          clock={clock}
          drawing={active}
          settings={settings}
          retainSeconds={liveRetainSeconds}
        />
      ),
    status,
    comments: listed,
    force,
    post,
  };
}

function noBroadcast(): BroadcastNow | null {
  return null;
}

// commentWait は、届いたコメントを、今から何秒後に流すかを返す。流さないなら null を返す。
// 書き込まれた時刻 postedAt と放送の時計があれば、場面に合わせる (#1515)。なければ、先読みの分だけ遅らせる。
function commentWait(
  postedAt: number | undefined,
  broadcast: BroadcastNow | null,
  video: HTMLVideoElement | null,
): number | null {
  if (postedAt === undefined || broadcast === null) {
    return playbackDelay(video);
  }

  return syncedCommentWait(postedAt, broadcast);
}
