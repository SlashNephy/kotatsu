import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';

import { CommentCanvas, type CommentCanvasHandle } from './CommentCanvas';
import { NXJikkyoSession, type SessionStatus } from './session';

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
export function useLiveJikkyo(
  channelId: string | null,
  videoRef: RefObject<HTMLVideoElement | null>,
  active: boolean,
): {
  overlay: ReactNode;
  status: SessionStatus | null;
  post: (text: string) => Promise<void>;
} {
  const canvas = useRef<CommentCanvasHandle | null>(null);
  // コメントを描く時計。画面を開いてからの秒。
  const origin = useRef(performance.now());
  const clock = () => (performance.now() - origin.current) / 1000;
  const sessionRef = useRef<NXJikkyoSession | null>(null);
  const [status, setStatus] = useState<SessionStatus | null>(null);

  useEffect(() => {
    if (channelId === null || !active) {
      return;
    }
    const session = new NXJikkyoSession(channelId, {
      onComment: (comment) => {
        const time = (comment.at - origin.current) / 1000 + playbackDelay(videoRef.current);
        canvas.current?.add([{ time, content: comment.content, mail: comment.mail }]);
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

  const post = async (text: string) => {
    const session = sessionRef.current;
    if (session === null) {
      throw new Error('not connected');
    }
    await session.post(text);
    // 自分の投稿は、届くのを待たずに、今の場面に流す。
    canvas.current?.add([{ time: clock(), content: text, mail: '184' }]);
  };

  return {
    overlay:
      channelId === null ? undefined : (
        <CommentCanvas ref={canvas} clock={clock} drawing={active} />
      ),
    status,
    post,
  };
}
