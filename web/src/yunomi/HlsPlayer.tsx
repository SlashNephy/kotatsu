import type Hls from 'hls.js';
import { useEffect, useRef, type RefObject } from 'react';

import classes from './TsPlayer.module.css';

type Props = {
  // url は、kyusu の HLS のプレイリストの URL。null なら、何も読まない。
  url: string | null;
  videoRef: RefObject<HTMLVideoElement | null>;
  // active は、再生してよいかどうか。画面の外に出たときや、タブが隠れたときに false にする。
  active: boolean;
  // live は、ライブ視聴かどうか。ライブは、隠れている間は読み込みをやめ、戻ったら最新の位置から読み直す。
  // 録画は、隠れている間は読み込みを止め、戻ったら同じ位置から続ける。
  live: boolean;
  // startPosition は、録画で、読み始める位置 (秒)。画質を変えたときに、それまでの位置から続けるのに使う。
  startPosition?: number;
  onError?: (error: Error) => void;
  // onLoaded は、プレイリストを読み、再生を始められるようになったときに呼ぶ。
  onLoaded?: () => void;
};

// HlsPlayer は、kyusu がトランスコードした HLS を hls.js で再生する (設計書 7.3)。
// iPhone の Safari では、ManagedMediaSource の上で hls.js が動く。
export function HlsPlayer({
  url,
  videoRef,
  active,
  live,
  startPosition,
  onError,
  onLoaded,
}: Props) {
  const hlsRef = useRef<Hls | null>(null);
  const handlers = useRef({ onError, onLoaded });
  handlers.current = { onError, onLoaded };
  // 読み始める位置は、読み直すときにだけ使う。位置が変わるたびに読み直さないよう、ref で渡す。
  const start = useRef(startPosition);
  start.current = startPosition;
  const frameRef = useRef<HTMLDivElement | null>(null);

  // video の要素は、TsPlayer と同じく React に持たせずに作り、どちらの経路でも同じ見た目にする。
  useEffect(() => {
    const frame = frameRef.current;
    if (frame === null) {
      return;
    }
    const video = document.createElement('video');
    video.className = classes.video ?? '';
    video.playsInline = true;
    video.autoplay = true;
    frame.append(video);
    videoRef.current = video;

    return () => {
      videoRef.current = null;
      frame.replaceChildren();
    };
  }, [videoRef]);

  // ライブは、隠れている間は hls.js ごと外し、kyusu のセッションを使わないようにする。
  const attached = live ? active : true;

  useEffect(() => {
    const video = videoRef.current;
    if (video === null || url === null || !attached) {
      return;
    }
    let destroyed = false;
    void import('hls.js').then(({ default: HlsClass }) => {
      if (destroyed) {
        return;
      }
      const hls = new HlsClass({ startPosition: live ? -1 : (start.current ?? -1) });
      hls.on(HlsClass.Events.MANIFEST_PARSED, () => {
        handlers.current.onLoaded?.();
        void video.play().catch(() => {});
      });
      hls.on(HlsClass.Events.ERROR, (_event, data) => {
        // 致命的でないエラーは、hls.js が読み直して回復する。
        if (data.fatal) {
          handlers.current.onError?.(new Error(`${data.type}: ${data.details}`));
        }
      });
      hls.loadSource(url);
      hls.attachMedia(video);
      hlsRef.current = hls;
    });

    return () => {
      destroyed = true;
      hlsRef.current?.destroy();
      hlsRef.current = null;
    };
  }, [videoRef, url, live, attached]);

  // 録画は、隠れている間は読み込みを止め、戻ったら同じ位置から続ける。
  useEffect(() => {
    const video = videoRef.current;
    const hls = hlsRef.current;
    if (live || video === null || hls === null) {
      return;
    }
    if (active) {
      hls.startLoad(video.currentTime);
      void video.play().catch(() => {});
    } else {
      video.pause();
      hls.stopLoad();
    }
  }, [videoRef, active, live]);

  return <div ref={frameRef} className={classes.frame} />;
}
