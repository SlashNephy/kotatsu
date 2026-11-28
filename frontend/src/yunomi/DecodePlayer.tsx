import { type ReactNode, type RefObject, useEffect, useRef } from 'react';

import { decodedAudioTracks, type PlayerAudioTracks } from './audio';
import { brightnessStyle } from './brightness';
import { type CaptionStyle, type Captions, createCaptions } from './captions';
import { type Crop, cropStyle, isCropped, noCrop, scales } from './crop';
import type { FallbackReason } from './decodeJudge';
import { DecodeMedia } from './decodeMedia';
import { dataBroadcastSource } from './route';
import type { DecodeDisplay, FromDecodeWorker, ToDecodeWorker } from './decodeProtocol';
import type { PlayerMedia } from './playerMedia';
import type { PlayerState } from '@mpeg2toh264/player';
import {
  useSuperResolution,
  useSuperResolutionMethod,
  useSuperResolutionStrength,
} from './superResolution';
import { containSize, deviceBox } from './videoOverlay';
import classes from './TsPlayer.module.css';

// wasmURL は、デコーダの WebAssembly の URL。Vite にワーカーへ埋め込ませず、明示的な URL で配信して、
// ストリーミングコンパイルを効かせる (設計書 7.2、#1516)。
const wasmURL = `${import.meta.env.BASE_URL}ffmpeg/decoder.wasm`;

type Props = {
  // url は、読む TS の URL。null なら読まない。URL の最後のサービス ID から、TS の中の番組を選ぶ。
  url: string | null;
  // mediaRef には、映している経路を入れる。
  mediaRef: RefObject<PlayerMedia | null>;
  onMedia?: (media: PlayerMedia | null) => void;
  captions: boolean;
  superimpose?: boolean;
  captionStyle?: CaptionStyle;
  crop?: Crop;
  brightness?: number;
  onCaptionText?: (text: string) => void;
  // onState は、読み終えたら completed を通知する。続けられなくなったことは、onError で通知する。
  onState?: (state: PlayerState) => void;
  onError?: (error: Error) => void;
  // onLoaded は、最初のコマを映せるようになったときに呼ぶ。
  onLoaded?: () => void;
  // onFallback は、この端末では間に合わない (または音を鳴らせない) と判定したときに呼ぶ。ほかの経路へ移すのは呼んだ側。
  onFallback?: (reason: FallbackReason) => void;
  // onResume は、止めた後に再開するときに呼ぶ。渡したら、呼んだ側が url を変えて読み直させる (#2166)。
  // 渡さなければ、同じ url を読み直す (ライブは、放送の今から)。
  onResume?: () => void;
  // onAudio は、番組の音声と、聞いているものが変わったら呼ぶ (#2165)。
  onAudio?: (tracks: PlayerAudioTracks | null) => void;
  // audio は、聞く音声 (audioChoices の値)。null なら、選び直さない。
  audio?: string | null;
  overlay?: ReactNode;
  controls?: ReactNode;
};

// DecodePlayer は、ブラウザの中で MPEG-2 をデコードする経路 (#1516、設計書 7.2) のプレイヤー。
// worker が WASM の FFmpeg でデコードして canvas に描き、DecodeMedia が音を鳴らす。DOM の形 (.media の中の .slot) は
// TsPlayer と同じにし、データ放送への映像の移し替えと、キャプチャが同じように扱えるようにする。
export function DecodePlayer({
  url,
  mediaRef,
  onMedia,
  captions,
  superimpose = true,
  captionStyle,
  crop = noCrop,
  brightness = 1,
  onCaptionText,
  onState,
  onError,
  onLoaded,
  onFallback,
  onResume,
  onAudio,
  audio = null,
  overlay,
  controls,
}: Props) {
  const boxRef = useRef<HTMLDivElement | null>(null);
  const handlers = useRef({
    onMedia,
    onState,
    onError,
    onLoaded,
    onCaptionText,
    onFallback,
    onResume,
    onAudio,
  });
  handlers.current = {
    onMedia,
    onState,
    onError,
    onLoaded,
    onCaptionText,
    onFallback,
    onResume,
    onAudio,
  };
  // 映像の拡大 (#1654、#2276) は、変換の経路と同じ端末ごとの設定で決める。worker は設定を読めないので、送って渡す。
  const [superResolution] = useSuperResolution();
  const [superResolutionStrength] = useSuperResolutionStrength();
  const [superResolutionMethod] = useSuperResolutionMethod();
  const zoom = scales(crop);
  const settingsRef = useRef<Omit<DecodeDisplay, 'box'>>({
    zoom,
    superResolution,
    strength: superResolutionStrength,
    method: superResolutionMethod,
  });
  settingsRef.current = {
    zoom,
    superResolution,
    strength: superResolutionStrength,
    method: superResolutionMethod,
  };
  const engineRef = useRef<{
    media: DecodeMedia;
    captions: Captions;
    worker: Worker;
    // sendDisplay は、今の大きさと設定を worker へ送る。
    sendDisplay: () => void;
  } | null>(null);
  // programNumber は、TS の中から選ぶ番組 (program_number)。分からなければ、最初の番組を選ぶ。
  const programNumber = url === null ? null : (dataBroadcastSource(url)?.serviceId ?? null);

  // canvas と worker は、React に持たせずに作る (canvas は worker へ渡すと、DOM から描けなくなるため)。
  useEffect(() => {
    const box = boxRef.current;
    if (box === null) {
      return;
    }
    const slot = document.createElement('div');
    slot.className = classes.slot ?? '';
    const canvas = document.createElement('canvas');
    canvas.className = classes.video ?? '';
    slot.append(canvas);
    box.append(slot);
    const worker = new Worker(new URL('./decodeWorker.ts', import.meta.url), { type: 'module' });
    const offscreen = canvas.transferControlToOffscreen();
    worker.postMessage(
      {
        type: 'start',
        canvas: offscreen,
        wasmUrl: wasmURL,
        timeOrigin: performance.timeOrigin,
      } satisfies ToDecodeWorker,
      [offscreen],
    );
    const media = new DecodeMedia({
      canvas,
      worker,
      resume: () => {
        const resume = handlers.current.onResume;
        if (resume === undefined) {
          return false;
        }
        resume();

        return true;
      },
    });
    const renderer = createCaptions(media, slot, (text) => handlers.current.onCaptionText?.(text));
    // 字幕の PES は、画面の時刻 (currentTime と同じ物差し) にして渡す。起点が決まるまではバッファする。
    // 読み直すと画面の時刻の起点が変わるので、バッファしたものと出ている字幕は、読み込みが替わったときに破棄する。
    // loadstart は遅れて届くので、それより先に新しい読み込みの字幕が届いても破棄しないよう、どの読み込みの分かを保持する。
    let waiting: { data: ArrayBuffer; pts: number }[] = [];
    let waitingLoad = media.currentLoad;
    const follow = (load: number) => {
      if (load === waitingLoad) {
        return;
      }
      waitingLoad = load;
      waiting = [];
      renderer.reset();
    };
    const flush = () => {
      const ready: { data: ArrayBuffer; pts: number }[] = [];
      for (const item of waiting) {
        const time = media.timeOf(item.pts);
        if (time === null) {
          return;
        }
        ready.push({ data: item.data, pts: time });
      }
      waiting = [];
      for (const item of ready) {
        renderer.feedB24(item.data, item.pts);
      }
    };
    const onMessage = (event: MessageEvent<FromDecodeWorker>) => {
      const message = event.data;
      if (message.type === 'fallback') {
        if (message.load === media.currentLoad) {
          handlers.current.onFallback?.(message.reason);
        }

        return;
      }
      if (message.type === 'audioTracks') {
        if (message.load === media.currentLoad) {
          handlers.current.onAudio?.(
            decodedAudioTracks(message.streams, message.current, message.dualMono),
          );
        }

        return;
      }
      // worker の失敗は DecodeMedia が error として送るので、ここでは字幕と起点だけを扱う。
      // 選局した後に、前の局の字幕を出さないよう、今の読み込みのものだけを使う。
      if (
        (message.type !== 'caption' && message.type !== 'origin') ||
        message.load !== media.currentLoad
      ) {
        return;
      }
      follow(message.load);
      if (message.type === 'caption') {
        waiting.push({ data: message.data, pts: message.pts });
      }
      // 起点は DecodeMedia が先に受け取る (先に listener を付けている) ので、起点より前に届いた字幕もここで出せる。
      flush();
    };
    worker.addEventListener('message', onMessage);
    // 拡大する大きさは、絵を枠に収めた矩形の、端末の画素での大きさで決める (変換の経路と同じ)。
    // canvas は枠いっぱいに置き、絵は CSS (object-fit: contain) で収めるので、枠の大きさを計測し、絵の比率で収める。
    let frame: { width: number; height: number } | null = null;
    let sent: string | null = null;
    const sendDisplay = () => {
      const picture = { width: media.videoWidth, height: media.videoHeight };
      const message: ToDecodeWorker = {
        type: 'display',
        box: frame === null ? null : containSize(picture, frame),
        ...settingsRef.current,
      };
      // 枠の大きさは、変わらなくても通知されることがあるので、同じものは送らない。
      const key = JSON.stringify(message);
      if (key !== sent) {
        sent = key;
        worker.postMessage(message);
      }
    };
    const resizes = new ResizeObserver(([entry]) => {
      if (entry === undefined) {
        return;
      }
      frame = deviceBox(
        { width: entry.contentRect.width, height: entry.contentRect.height },
        entry.devicePixelContentBoxSize?.[0],
        window.devicePixelRatio || 1,
      );
      sendDisplay();
    });
    try {
      resizes.observe(canvas, { box: 'device-pixel-content-box' });
    } catch {
      // Safari は端末の画素の大きさを通知しないので、CSS の大きさに画素の比率を掛ける (deviceBox)。
      resizes.observe(canvas);
    }
    const onMediaError = () =>
      handlers.current.onError?.(new Error(media.error?.message ?? 'decoding failed'));
    const onEnded = () => handlers.current.onState?.('completed');
    const onLoadedData = () => handlers.current.onLoaded?.();
    // 字幕のない局へ移ったときも、前の局の字幕を消す。
    const onLoadStart = () => follow(media.currentLoad);
    media.addEventListener('error', onMediaError);
    media.addEventListener('ended', onEnded);
    media.addEventListener('loadeddata', onLoadedData);
    media.addEventListener('loadstart', onLoadStart);
    // 映す大きさが変わると、絵を収めた矩形も変わる。
    media.addEventListener('resize', sendDisplay);
    engineRef.current = { media, captions: renderer, worker, sendDisplay };
    mediaRef.current = media;
    handlers.current.onMedia?.(media);

    return () => {
      media.removeEventListener('error', onMediaError);
      media.removeEventListener('ended', onEnded);
      media.removeEventListener('loadeddata', onLoadedData);
      media.removeEventListener('loadstart', onLoadStart);
      media.removeEventListener('resize', sendDisplay);
      resizes.disconnect();
      worker.removeEventListener('message', onMessage);
      renderer.destroy();
      media.dispose();
      worker.terminate();
      engineRef.current = null;
      mediaRef.current = null;
      handlers.current.onMedia?.(null);
      handlers.current.onAudio?.(null);
      slot.remove();
    };
  }, [mediaRef]);

  useEffect(() => {
    const engine = engineRef.current;
    if (engine === null) {
      return;
    }
    if (url === null) {
      engine.media.pause();

      return;
    }
    engine.media.open(url, programNumber);
    // 自動再生を拒否されたら、止まったままにし、ユーザーの操作を待つ (useAutoplayBlocked が再生ボタンを出す)。
    void engine.media.play().catch(() => {});
  }, [url, programNumber]);

  // 聞く音声を worker に選ばせる (#2165)。worker は、読み込み直しても選んだものを保つ。
  useEffect(() => {
    const message = audioSelection(audio);
    if (message !== null) {
      engineRef.current?.worker.postMessage(message);
    }
  }, [audio]);

  useEffect(() => {
    engineRef.current?.sendDisplay();
  }, [superResolution, superResolutionStrength, superResolutionMethod, zoom.x, zoom.y]);

  useEffect(() => {
    engineRef.current?.captions.setVisible(captions);
  }, [captions]);

  useEffect(() => {
    engineRef.current?.captions.setSuperimposeVisible(superimpose);
  }, [superimpose]);

  useEffect(() => {
    if (captionStyle !== undefined) {
      engineRef.current?.captions.setStyle(captionStyle);
    }
  }, [captionStyle]);

  return (
    <div className={classes.frame}>
      <div
        className={classes.clip}
        data-fill={crop.fill === true && isCropped(crop) ? '' : undefined}
      >
        <div
          ref={boxRef}
          className={classes.media}
          style={{ ...cropStyle(crop), ...brightnessStyle(brightness) }}
        />
      </div>
      {overlay !== undefined && <div className={classes.overlay}>{overlay}</div>}
      {controls}
    </div>
  );
}

// audioSelection は、audioChoices の値を、worker に送る音声の選択にする。選び直さない値なら null を返す。
function audioSelection(choice: string | null): ToDecodeWorker | null {
  if (choice === null) {
    return null;
  }
  const [kind, value] = choice.split(':');
  if (kind === 'stream' && value !== undefined && /^\d+$/.test(value)) {
    return { type: 'selectAudio', pid: Number(value) };
  }
  if (kind === 'dualMono' && (value === 'main' || value === 'sub' || value === 'both')) {
    return { type: 'selectAudio', dualMono: value };
  }

  return null;
}
