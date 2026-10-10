import type Hls from 'hls.js';
import { useEffect, useRef, useState, type RefObject } from 'react';

import { createCaptions, type Captions } from './captions';
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
  // captions は、字幕と文字スーパーを出すかどうか。
  captions: boolean;
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
  captions,
  startPosition,
  onError,
  onLoaded,
}: Props) {
  const hlsRef = useRef<Hls | null>(null);
  // hls は、作り終えた hls.js。作るのを待つ間に active が変わっても、作り終えてから読み込みを止められるようにする。
  const [hls, setHls] = useState<Hls | null>(null);
  const activeRef = useRef(active);
  activeRef.current = active;
  // resume は、隠れる前に再生していたかどうか。利用者が止めていた録画は、戻っても止めたままにする。
  const resume = useRef(true);
  // loading は、hls.js がセグメントを読んでいるかどうか。読み込みを止めたり再開したりするのは、変わったときだけにする。
  const loading = useRef(false);
  const handlers = useRef({ onError, onLoaded });
  handlers.current = { onError, onLoaded };
  // 読み始める位置は、読み直すときにだけ使う。位置が変わるたびに読み直さないよう、ref で渡す。
  const start = useRef(startPosition);
  start.current = startPosition;
  const frameRef = useRef<HTMLDivElement | null>(null);
  const captionsRef = useRef<Captions | null>(null);
  const showCaptions = useRef(captions);
  showCaptions.current = captions;

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
    let destroyed = false;
    void createCaptions(video, frame).then((renderer) => {
      if (destroyed) {
        renderer.destroy();

        return;
      }
      renderer.setVisible(showCaptions.current);
      captionsRef.current = renderer;
    });

    return () => {
      destroyed = true;
      captionsRef.current?.destroy();
      captionsRef.current = null;
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
      const instance = new HlsClass({
        startPosition: live ? -1 : (start.current ?? -1),
        // 隠れたまま開いた録画は、見えるまでセグメントを読まない。
        autoStartLoad: activeRef.current,
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
        if (activeRef.current) {
          void video.play().catch(() => {});
        }
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
      loading.current = activeRef.current;
      setHls(instance);
    });

    return () => {
      destroyed = true;
      hlsRef.current?.destroy();
      hlsRef.current = null;
      setHls(null);
    };
  }, [videoRef, url, live, attached]);

  useEffect(() => {
    captionsRef.current?.setVisible(captions);
  }, [captions]);

  // 録画は、隠れている間は読み込みを止め、戻ったら同じ位置から続ける。
  useEffect(() => {
    const video = videoRef.current;
    if (live || video === null || hls === null || active === loading.current) {
      return;
    }
    loading.current = active;
    if (active) {
      // 一度も読んでいなければ、読み始める位置から読む。
      hls.startLoad(video.readyState > 0 ? video.currentTime : (start.current ?? -1));
      if (resume.current) {
        void video.play().catch(() => {});
      }
    } else {
      resume.current = !video.paused;
      video.pause();
      hls.stopLoad();
    }
  }, [videoRef, hls, active, live]);

  return <div ref={frameRef} className={classes.frame} />;
}
