import type { AudioTracks, Mpeg2TsPlayer, PlayerState, PrivateStream } from '@mpeg2toh264/player';
import { useEffect, useRef, type ReactNode, useState, type RefObject } from 'react';

import { audioChoices, currentAudioChoice } from './audio';
import { cropStyle, noCrop, type Crop } from './crop';
import { createCaptions, type Captions } from './captions';
import { defaultCaptionStyle, type CaptionStyle } from './captionStyle';
import { deinterlacerCanvas } from './deinterlacer';
import classes from './TsPlayer.module.css';

// wasmURL は、変換に使う WebAssembly の URL。Vite にワーカーへ埋め込ませず、明示的な URL で配信して、
// ストリーミングコンパイルを効かせる (設計書 7.2)。
const wasmURL = '/mpeg2toh264/mpeg2toh264_wasm_bg.wasm';

type Props = {
  // url は、流す TS の URL。null なら、何も読まない。
  url: string | null;
  videoRef: RefObject<HTMLVideoElement | null>;
  // captions は、字幕と文字スーパーを出すかどうか。
  captions: boolean;
  // superimpose は、文字スーパーを出すかどうか (#491)。captions を切っていれば、出さない。
  superimpose?: boolean;
  // captionStyle は、字幕の見た目 (#491)。
  captionStyle?: CaptionStyle;
  // crop は、L 字放送の切り抜き (#496)。映像と字幕をまとめて拡大する。
  crop?: Crop;
  // onCaptionText は、字幕が出るたびに、その文を受け取る (#450)。字幕を出さない設定でも受け取る。
  onCaptionText?: (text: string) => void;
  onState?: (state: PlayerState) => void;
  onError?: (error: Error) => void;
  // onLoaded は、最初のデータが入り、再生を始められるようになったときに呼ぶ。
  onLoaded?: () => void;
  // overlay は、映像の上に重ねるもの (実況のコメント)。
  overlay?: ReactNode;
  // controls は、映像の上に重ねる操作バー。重ねたもの (overlay) より上に置き、操作を受ける。
  controls?: ReactNode;
  // onVideo は、video の要素を作ったときと、取り除いたとき (null) に呼ぶ。
  onVideo?: (video: HTMLVideoElement | null) => void;
  // onAudio は、番組の音声と、今どれを聞いているかが分かったとき、変わったときに呼ぶ。
  onAudio?: (tracks: AudioTracks) => void;
  // audio は、聞く音声 (audioChoices の値)。null なら、プレイヤーに任せる。
  audio?: string | null;
};

// TsPlayer は、MPEG-2 の TS をブラウザの中で H.264 に変換して再生する (設計書 7.2)。
// 1080i のインターレースは、yadif で解除する。yadif は LGPL なので、別のチャンクとして読む。
export function TsPlayer({
  url,
  videoRef,
  captions,
  superimpose = true,
  captionStyle = defaultCaptionStyle,
  crop = noCrop,
  onCaptionText,
  onState,
  onError,
  onLoaded,
  overlay,
  controls,
  onVideo,
  onAudio,
  audio = null,
}: Props) {
  const playerRef = useRef<Mpeg2TsPlayer | null>(null);
  const captionsRef = useRef<Captions | null>(null);
  const showCaptions = useRef(captions);
  // captionText は、字幕の文を渡す先。字幕は作り直さずに、渡す先だけを差し替える。
  const captionText = useRef(onCaptionText);
  captionText.current = onCaptionText;
  showCaptions.current = captions;
  // 字幕を作り直したときに、今の設定から始めるよう、ref でも持つ。
  const superimposeRef = useRef(superimpose);
  superimposeRef.current = superimpose;
  const styleRef = useRef(captionStyle);
  styleRef.current = captionStyle;
  const [ready, setReady] = useState(false);
  const handlers = useRef({ onState, onError, onLoaded, onAudio, onVideo });
  handlers.current = { onState, onError, onLoaded, onAudio, onVideo };
  const audioRef = useRef(audio);
  audioRef.current = audio;

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
    handlers.current.onVideo?.(video);
    // 字幕は、aribb24.js を読み終えるまで溜めておくので、プレイヤーより先に作る。
    const renderer = createCaptions(video, frame, (text) => captionText.current?.(text));
    renderer.setVisible(showCaptions.current);
    renderer.setSuperimposeVisible(superimposeRef.current);
    renderer.setStyle(styleRef.current);
    captionsRef.current = renderer;
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
        deinterlacer: (element) => {
          const deinterlacer = new Deinterlacer(element);
          // yadif は、解除した映像を描く canvas の位置と大きさを、video に合わせて自分で決める。
          // データ放送の中へ映像を移すときに、この canvas の大きさを変えないよう印を付ける。
          deinterlacer.canvas.dataset[deinterlacerCanvas] = '';

          return deinterlacer;
        },
      });
      player.addEventListener('statechange', (event) =>
        handlers.current.onState?.(event.detail.state),
      );
      player.addEventListener('error', (event) => handlers.current.onError?.(event.detail.error));
      // 字幕は private_stream_1、文字スーパーは private_stream_2 で届く。時刻は、video の時刻にそろっている。
      const feed = (event: CustomEvent<PrivateStream>) => {
        if (event.detail.pts !== null) {
          renderer.feedB24(event.detail.data, event.detail.pts);
        }
      };
      player.addEventListener('audio', (event) => {
        // 読み直すと、プレイヤーは、新しいストリームの音声が分かったところで既定の音声に戻る。
        // 選んでいた音声がそのストリームにあれば、選び直す。選び直すと、もう一度このイベントが届く。
        const wanted = audioRef.current;
        if (
          wanted !== null &&
          wanted !== currentAudioChoice(event.detail) &&
          audioChoices(event.detail).some((choice) => choice.value === wanted)
        ) {
          selectAudio(player, wanted);
        }
        handlers.current.onAudio?.(event.detail);
      });
      player.addEventListener('private_stream_1', feed);
      player.addEventListener('private_stream_2', feed);
      playerRef.current = player;
      setReady(true);
    })();

    return () => {
      destroyed = true;
      playerRef.current?.destroy();
      playerRef.current = null;
      captionsRef.current?.destroy();
      captionsRef.current = null;
      videoRef.current = null;
      handlers.current.onVideo?.(null);
      // yadif が包んだ要素ごと取り除く。
      frame.replaceChildren();
      setReady(false);
    };
  }, [videoRef]);

  useEffect(() => {
    captionsRef.current?.setVisible(captions);
  }, [captions]);

  useEffect(() => {
    captionsRef.current?.setSuperimposeVisible(superimpose);
  }, [superimpose]);

  const { stroke, backgroundOpacity } = captionStyle;
  useEffect(() => {
    captionsRef.current?.setStyle({ stroke, backgroundOpacity });
  }, [stroke, backgroundOpacity]);

  // 聞く音声を選ぶ。切り替えは、これから変換する部分から効く (設計書 12 章のデュアルモノ)。
  // 読み直すとプレイヤーが既定の音声に戻ることがあるので、読み直すたびにも選び直す。
  useEffect(() => {
    const player = playerRef.current;
    if (ready && player !== null) {
      selectAudio(player, audio);
    }
  }, [ready, audio]);

  // URL が変わったら読み直す。URL がなければ、読み込みを止めて、帯域とチューナーを使わない。
  useEffect(() => {
    const player = playerRef.current;
    if (!ready || player === null) {
      return;
    }
    if (url === null) {
      player.stop();

      return;
    }
    let cancelled = false;
    captionsRef.current?.reset();
    selectAudio(player, audioRef.current);
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
      // URL が変わるか、外すときは、前の読み込みを止めて、サーバーに無駄な読み込みをさせない。
      player.stop();
    };
  }, [ready, url]);

  return (
    <div className={classes.frame}>
      {/* 切り抜き (#496) で拡大した映像と字幕を、枠の外に出さない。 */}
      <div className={classes.clip}>
        <div ref={frameRef} className={classes.media} style={cropStyle(crop)} />
      </div>
      {overlay !== undefined && <div className={classes.overlay}>{overlay}</div>}
      {controls}
    </div>
  );
}

// selectAudio は、audioChoices の値 choice の音声を、プレイヤーに選ばせる。null なら、何もしない。
function selectAudio(player: Mpeg2TsPlayer, choice: string | null): void {
  if (choice === null) {
    return;
  }
  const [kind, value] = choice.split(':');
  if (kind === 'stream') {
    player.selectAudio(Number(value));
  } else if (kind === 'dualMono') {
    player.selectDualMono(value === 'sub');
  }
}
