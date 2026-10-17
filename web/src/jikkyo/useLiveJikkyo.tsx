import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';

import { appendComment, type ListedComment } from './commentList';
import { CommentCanvas, type CommentCanvasHandle } from './CommentCanvas';
import { compileMute, defaultCommentSettings, type CommentSettings } from './comments';
import { NXJikkyoSession, type SessionStatus } from './session';

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
export function useLiveJikkyo(
  channelId: string | null,
  videoRef: RefObject<HTMLVideoElement | null>,
  active: boolean,
  settings: CommentSettings = defaultCommentSettings,
): {
  overlay: ReactNode;
  status: SessionStatus | null;
  // comments は、届いたコメントの一覧 (#489)。新しいものが末尾。
  comments: readonly ListedComment[];
  post: (text: string) => Promise<void>;
} {
  const canvas = useRef<CommentCanvasHandle | null>(null);
  // コメントを描く時計。画面を開いてからの秒。
  const origin = useRef(performance.now());
  const clock = () => (performance.now() - origin.current) / 1000;
  const sessionRef = useRef<NXJikkyoSession | null>(null);
  const [status, setStatus] = useState<SessionStatus | null>(null);
  const [comments, setComments] = useState<readonly ListedComment[]>([]);
  // ミュートは、届いたときの設定で決める。接続し直さないよう、ref で読む。
  const muted = useRef(compileMute(settings));
  muted.current = compileMute(settings);

  useEffect(() => {
    if (channelId === null || !active) {
      return;
    }
    const session = new NXJikkyoSession(channelId, {
      onComment: (comment) => {
        const time = (comment.at - origin.current) / 1000 + playbackDelay(videoRef.current);
        const drawn = {
          time,
          content: comment.content,
          mail: comment.mail,
          ...(comment.userId === undefined ? {} : { userId: comment.userId }),
        };
        canvas.current?.add([drawn]);
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

    return () => {
      session.close();
      sessionRef.current = null;
      setStatus(null);
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
    comments,
    post,
  };
}
