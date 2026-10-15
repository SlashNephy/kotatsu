import type Hls from 'hls.js';
import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';

import { createCaptions, type Captions } from './captions';
import classes from './TsPlayer.module.css';

type Props = {
  // url は、kyusu の HLS のプレイリストの URL。null なら、何も読まない。
  url: string | null;
  videoRef: RefObject<HTMLVideoElement | null>;
  // live は、ライブ視聴かどうか。ライブは最新の位置から、録画は startPosition から読む。
  live: boolean;
  // captions は、字幕と文字スーパーを出すかどうか。
  captions: boolean;
  // startPosition は、録画で、読み始める位置 (秒)。画質を変えたときに、それまでの位置から続けるのに使う。
  startPosition?: number;
  onError?: (error: Error) => void;
  // onLoaded は、プレイリストを読み、再生を始められるようになったときに呼ぶ。
  onLoaded?: () => void;
  // overlay は、映像の上に重ねるもの (実況のコメント)。
  overlay?: ReactNode;
  // controls は、映像の上に重ねる操作バー。重ねたもの (overlay) より上に置き、操作を受ける。
  controls?: ReactNode;
  // onVideo は、video の要素を作ったときと、取り除いたとき (null) に呼ぶ。
  onVideo?: (video: HTMLVideoElement | null) => void;
};

// liveResumeLimitMs は、ライブを一時停止してから、続きから読めるとみなす時間。
// kyusu は、セグメントの要求が 10 秒途絶えたらセッションを止める (設計書 7.3) ので、それより短くする。
const liveResumeLimitMs = 8_000;

// HlsPlayer は、kyusu がトランスコードした HLS を hls.js で再生する (設計書 7.3)。
// iPhone の Safari では、ManagedMediaSource の上で hls.js が動く。
export function HlsPlayer({
  url,
  videoRef,
  live,
  captions,
  startPosition,
  onError,
  onLoaded,
  overlay,
  controls,
  onVideo,
}: Props) {
  const hlsRef = useRef<Hls | null>(null);
  const handlers = useRef({ onError, onLoaded, onVideo });
  handlers.current = { onError, onLoaded, onVideo };
  // 読み始める位置は、読み直すときにだけ使う。位置が変わるたびに読み直さないよう、ref で渡す。
  const start = useRef(startPosition);
  start.current = startPosition;
  const frameRef = useRef<HTMLDivElement | null>(null);
  const captionsRef = useRef<Captions | null>(null);
  const showCaptions = useRef(captions);
  showCaptions.current = captions;
  // generation を変えると、hls.js を作り直して読み直す。ライブを長く一時停止してから再開したときに使う。
  const [generation, setGeneration] = useState(0);

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
    handlers.current.onVideo?.(video);
    // 字幕は、aribb24.js を読み終えるまで溜めておくので、最初のセグメントの字幕も落とさない。
    const renderer = createCaptions(video, frame);
    renderer.setVisible(showCaptions.current);
    captionsRef.current = renderer;

    return () => {
      captionsRef.current?.destroy();
      captionsRef.current = null;
      videoRef.current = null;
      handlers.current.onVideo?.(null);
      frame.replaceChildren();
    };
  }, [videoRef]);

  useEffect(() => {
    const video = videoRef.current;
    if (video === null || url === null) {
      return;
    }
    let destroyed = false;
    void import('hls.js').then(({ default: HlsClass }) => {
      if (destroyed) {
        return;
      }
      const instance = new HlsClass({
        startPosition: live ? -1 : (start.current ?? -1),
        // 字幕は、FRAG_PARSING_METADATA で受け取った ID3 から描く。hls.js の字幕と、ID3 を入れる TextTrack は使わない。
        // TextTrack は、CSP で禁じた data: の URL を読もうとする。
        enableID3MetadataCues: false,
        enableEmsgMetadataCues: false,
        enableDateRangeMetadataCues: false,
        enableWebVTT: false,
        enableCEA708Captions: false,
      });
      // tsreadex が、字幕と文字スーパーを ID3 に入れて運ぶ (設計書 7.3)。時刻は、video の時刻にそろっている。
      instance.on(HlsClass.Events.FRAG_PARSING_METADATA, (_event, data) => {
        for (const sample of data.samples) {
          captionsRef.current?.feedID3(sample.data, sample.pts);
        }
      });
      instance.on(HlsClass.Events.MANIFEST_PARSED, () => {
        handlers.current.onLoaded?.();
        void video.play().catch(() => {});
      });
      instance.on(HlsClass.Events.ERROR, (_event, data) => {
        // 致命的でないエラーは、hls.js が読み直して回復する。
        if (data.fatal) {
          handlers.current.onError?.(new Error(`${data.type}: ${data.details}`));
        }
      });
      // ライブを読み直すと、video の時刻の基準が変わるので、前の字幕を捨てる。
      captionsRef.current?.reset();
      instance.loadSource(url);
      instance.attachMedia(video);
      hlsRef.current = instance;
    });

    // ライブを一時停止したら、プレイリストもセグメントも読まない (#534)。読み続けると、kyusu はセグメントを要求されないので
    // セッションを止め、プレイリストの要求でまたチューナーと FFmpeg を起動する、を繰り返す。
    // すぐに再開したら続きから読み、長く止めていたら、kyusu がセッションを止めているので、最新の位置から読み直す。
    let pausedAt: number | null = null;
    const pause = () => {
      if (!live) {
        return;
      }
      pausedAt = Date.now();
      hlsRef.current?.stopLoad();
    };
    const play = () => {
      if (pausedAt === null) {
        return;
      }
      const paused = Date.now() - pausedAt;
      pausedAt = null;
      if (paused < liveResumeLimitMs) {
        hlsRef.current?.startLoad();
      } else {
        setGeneration((n) => n + 1);
      }
    };
    video.addEventListener('pause', pause);
    video.addEventListener('play', play);

    return () => {
      destroyed = true;
      video.removeEventListener('pause', pause);
      video.removeEventListener('play', play);
      hlsRef.current?.destroy();
      hlsRef.current = null;
    };
  }, [videoRef, url, live, generation]);

  useEffect(() => {
    captionsRef.current?.setVisible(captions);
  }, [captions]);

  return (
    <div className={classes.frame}>
      <div ref={frameRef} className={classes.media} />
      {overlay !== undefined && <div className={classes.overlay}>{overlay}</div>}
      {controls}
    </div>
  );
}
