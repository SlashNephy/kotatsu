import type { Mpeg2TsPlayer, PlayerState } from '@mpeg2toh264/player';
import { useEffect, useRef, useState, type RefObject } from 'react';

import classes from './TsPlayer.module.css';

// wasmURL は、変換に使う WebAssembly の URL。Vite にワーカーへ埋め込ませず、明示的な URL で配信して、
// ストリーミングコンパイルを効かせる (設計書 7.2)。
const wasmURL = '/mpeg2toh264/mpeg2toh264_wasm_bg.wasm';

type Props = {
  // url は、流す TS の URL。null なら、何も読まない。
  url: string | null;
  videoRef: RefObject<HTMLVideoElement | null>;
  // active は、再生してよいかどうか。画面の外に出たときや、タブが隠れたときに false にする。
  active: boolean;
  onState?: (state: PlayerState) => void;
  onError?: (error: Error) => void;
  // onLoaded は、最初のデータが入り、再生を始められるようになったときに呼ぶ。
  onLoaded?: () => void;
};

// TsPlayer は、MPEG-2 の TS をブラウザの中で H.264 に変換して再生する (設計書 7.2)。
// 1080i のインターレースは、yadif で解除する。yadif は LGPL なので、別のチャンクとして読む。
export function TsPlayer({ url, videoRef, active, onState, onError, onLoaded }: Props) {
  const playerRef = useRef<Mpeg2TsPlayer | null>(null);
  const [ready, setReady] = useState(false);
  const handlers = useRef({ onState, onError, onLoaded });
  handlers.current = { onState, onError, onLoaded };

  const frameRef = useRef<HTMLDivElement | null>(null);

  // video の要素は、React に持たせずに作る。yadif は、video を別の要素で包んで動かすので、
  // React が持っていると、外すときに元の親から取り除こうとして失敗する。
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
    void (async () => {
      const [{ Mpeg2TsPlayer: Player }, { Deinterlacer }] = await Promise.all([
        import('@mpeg2toh264/player'),
        import('@mpeg2toh264/yadif'),
      ]);
      if (destroyed) {
        return;
      }
      const player = new Player(video, {
        wasmUrl: wasmURL,
        deinterlace: true,
        deinterlacer: (element) => new Deinterlacer(element),
      });
      player.addEventListener('statechange', (event) =>
        handlers.current.onState?.(event.detail.state),
      );
      player.addEventListener('error', (event) => handlers.current.onError?.(event.detail.error));
      playerRef.current = player;
      setReady(true);
    })();

    return () => {
      destroyed = true;
      playerRef.current?.destroy();
      playerRef.current = null;
      videoRef.current = null;
      // yadif が包んだ要素ごと取り除く。
      frame.replaceChildren();
      setReady(false);
    };
  }, [videoRef]);

  // URL が変わったら読み直す。再生してよくないときは、読み込みも止めて、帯域とチューナーを使わない。
  useEffect(() => {
    const player = playerRef.current;
    if (!ready || player === null) {
      return;
    }
    if (url === null || !active) {
      player.stop();

      return;
    }
    let cancelled = false;
    player.load(url).then(
      () => {
        if (!cancelled) {
          handlers.current.onLoaded?.();
        }
      },
      // 失敗は error のイベントでも届くので、ここでは捨てる。
      () => {},
    );

    return () => {
      cancelled = true;
    };
  }, [ready, url, active]);

  return <div ref={frameRef} className={classes.frame} />;
}
