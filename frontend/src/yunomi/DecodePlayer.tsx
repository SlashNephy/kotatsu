import { type ReactNode, type RefObject, useEffect, useRef } from 'react';

import { brightnessStyle } from './brightness';
import { type CaptionStyle, type Captions, createCaptions } from './captions';
import { type Crop, cropStyle, isCropped, noCrop } from './crop';
import { DecodeMedia } from './decodeMedia';
import { dataBroadcastSource } from './route';
import type { FromDecodeWorker, ToDecodeWorker } from './decodeProtocol';
import type { PlayerMedia } from './playerMedia';
import type { PlayerState } from '@mpeg2toh264/player';
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
  overlay,
  controls,
}: Props) {
  const boxRef = useRef<HTMLDivElement | null>(null);
  const handlers = useRef({ onMedia, onState, onError, onLoaded, onCaptionText });
  handlers.current = { onMedia, onState, onError, onLoaded, onCaptionText };
  const engineRef = useRef<{ media: DecodeMedia; captions: Captions } | null>(null);
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
    const media = new DecodeMedia({ canvas, worker });
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
    engineRef.current = { media, captions: renderer };
    mediaRef.current = media;
    handlers.current.onMedia?.(media);

    return () => {
      media.removeEventListener('error', onMediaError);
      media.removeEventListener('ended', onEnded);
      media.removeEventListener('loadeddata', onLoadedData);
      media.removeEventListener('loadstart', onLoadStart);
      worker.removeEventListener('message', onMessage);
      renderer.destroy();
      media.dispose();
      worker.terminate();
      engineRef.current = null;
      mediaRef.current = null;
      handlers.current.onMedia?.(null);
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
    // 自動再生を断られたら、止まったままにし、ユーザーの操作を待つ (useAutoplayBlocked が再生ボタンを出す)。
    void engine.media.play().catch(() => {});
  }, [url, programNumber]);

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
