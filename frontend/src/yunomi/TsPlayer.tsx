import type { AudioTracks, Mpeg2TsPlayer, PlayerState, PrivateStream } from '@mpeg2toh264/player';
import { useEffect, useRef, type ReactNode, useState, type RefObject } from 'react';

import { audioChoices, currentAudioChoice } from './audio';
import { cropStyle, noCrop, type Crop } from './crop';
import { createCaptions, type Captions } from './captions';
import { defaultCaptionStyle, type CaptionStyle } from './captionStyle';
import { deinterlacerCanvas } from './deinterlacer';
import { fitMedia } from './mediaFit';
import classes from './TsPlayer.module.css';

// wasmURL は、変換に使う WebAssembly の URL。Vite にワーカーへ埋め込ませず、明示的な URL で配信して、
// ストリーミングコンパイルを効かせる (設計書 7.2)。
const wasmURL = `${import.meta.env.BASE_URL}mpeg2toh264/mpeg2toh264_wasm_bg.wasm`;

// stallTimeoutMs は、再生しているのに、映像がこれより長く進まなければ、配信が切れたとみなす時間 (#1083)。
// 変換のプレイヤーは、読むのを止めている間に接続が切れると、エラーも終わりも知らせずに待ち続けるので、映像の進みで見張る。
const stallTimeoutMs = 5_000;

type Props = {
  // url は、流す TS の URL。null なら、何も読まない。
  url: string | null;
  // handover は、URL が変わったときに、今の映像を流したまま裏で読み込み、映像が出てから切り替えるかどうか (#1083)。
  handover?: boolean;
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

// Slot は、映像を 1 本読み込むための、video と変換のプレイヤーと字幕の組。切り替えのときだけ、裏でもう 1 つ作る (#1083)。
type Slot = {
  // container は、video と字幕の canvas を入れる要素。yadif が video を包む要素も、この中に入る。
  container: HTMLDivElement;
  video: HTMLVideoElement;
  captions: Captions;
  // player は、変換のプレイヤー。モジュールを読み終えるまでは null。
  player: Mpeg2TsPlayer | null;
  destroyed: boolean;
};

// loadModules は、変換のプレイヤーと yadif を読む。yadif は LGPL なので、別のチャンクとして読む。
const loadModules = () =>
  Promise.all([import('@mpeg2toh264/player'), import('@mpeg2toh264/yadif')] as const);

// TsPlayer は、MPEG-2 の TS をブラウザの中で H.264 に変換して再生する (設計書 7.2)。
// 1080i のインターレースは、yadif で解除する。
//
// handover なら、URL が変わったときに、今の映像を流したまま、裏で新しい URL を読み込み、映像が出てから切り替える (#1083)。
// つなぎ直しで、映像が途切れたり黒くなったりしないようにする。今の映像が出ていなければ (切れて空になったときなど)、すぐ読み直す。
export function TsPlayer({
  url,
  handover = false,
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
  // active は、映っている組。pending は、切り替えるために裏で読み込んでいる組。
  const activeRef = useRef<Slot | null>(null);
  const pendingRef = useRef<Slot | null>(null);
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
  // handover は、URL が変わったときにだけ読む。変えただけで読み直さないよう、ref で持つ。
  const handoverRef = useRef(handover);
  handoverRef.current = handover;

  const mediaRef = useRef<HTMLDivElement | null>(null);

  // createSlot は、組を作って media に入れる。hidden なら、切り替えるまで見せず、音も出さない。
  // video の要素は、React に持たせずに作る。yadif は、video を別の要素で包んで動かすので、
  // React が持っていると、外すときに元の親から取り除こうとして失敗する。
  // 変換のプレイヤーのイベントは、映っている組のものだけを渡す。裏で読み込んでいる組の失敗は、load の失敗で受け取る。
  const createSlot = (media: HTMLElement, hidden: boolean): Slot => {
    const container = document.createElement('div');
    container.className = classes.slot ?? '';
    // データ放送は、映像の要素を外の CSS が届かない所へ移すので、隠すのはインラインで指定する。
    container.style.visibility = hidden ? 'hidden' : '';
    const video = document.createElement('video');
    video.className = classes.video ?? '';
    video.playsInline = true;
    video.autoplay = true;
    video.muted = hidden;
    container.append(video);
    media.append(container);
    // 字幕は、aribb24.js を読み終えるまで溜めておくので、プレイヤーより先に作る。
    const renderer = createCaptions(video, container, (text) => {
      if (activeRef.current === slot) {
        captionText.current?.(text);
      }
    });
    renderer.setVisible(showCaptions.current);
    renderer.setSuperimposeVisible(superimposeRef.current);
    renderer.setStyle(styleRef.current);
    const slot: Slot = { container, video, captions: renderer, player: null, destroyed: false };

    return slot;
  };

  // attachPlayer は、組の変換のプレイヤーを作る。モジュールを読み終えるまで待つ。
  const attachPlayer = async (slot: Slot): Promise<Mpeg2TsPlayer | null> => {
    const [{ Mpeg2TsPlayer: Player }, { Deinterlacer }] = await loadModules();
    if (slot.destroyed) {
      return null;
    }
    const isActive = () => activeRef.current === slot;
    const player = new Player(slot.video, {
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
    player.addEventListener('statechange', (event) => {
      if (isActive()) {
        handlers.current.onState?.(event.detail.state);
      }
    });
    player.addEventListener('error', (event) => {
      if (isActive()) {
        handlers.current.onError?.(event.detail.error);
      }
    });
    // 字幕は private_stream_1、文字スーパーは private_stream_2 で届く。時刻は、video の時刻にそろっている。
    const feed = (event: CustomEvent<PrivateStream>) => {
      if (event.detail.pts !== null) {
        slot.captions.feedB24(event.detail.data, event.detail.pts);
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
      if (isActive()) {
        handlers.current.onAudio?.(event.detail);
      }
    });
    player.addEventListener('private_stream_1', feed);
    player.addEventListener('private_stream_2', feed);
    slot.player = player;

    return player;
  };

  // destroySlot は、組を捨てる。yadif が包んだ要素ごと取り除く。
  const destroySlot = (slot: Slot) => {
    slot.destroyed = true;
    slot.player?.destroy();
    slot.player = null;
    slot.captions.destroy();
    slot.container.remove();
  };

  // promote は、裏で読み込んだ組を映し、それまで映っていた組を捨てる。音量と再生の速さは、前の映像から引き継ぐ。
  const promote = (slot: Slot) => {
    const previous = activeRef.current;
    if (previous !== null) {
      slot.video.volume = previous.video.volume;
      slot.video.muted = previous.video.muted;
      slot.video.playbackRate = previous.video.playbackRate;
    } else {
      slot.video.muted = false;
    }
    slot.container.style.visibility = '';
    // データ放送が、映っていた組を自分の中へ移しているなら、新しい組を同じ所に置く。外の CSS が届かないので、大きさをインラインで整える。
    if (previous !== null && previous.container.parentNode !== mediaRef.current) {
      fitMedia([slot.container]);
      previous.container.replaceWith(slot.container);
    }
    pendingRef.current = null;
    activeRef.current = slot;
    videoRef.current = slot.video;
    handlers.current.onVideo?.(slot.video);
    if (previous !== null) {
      destroySlot(previous);
    }
    const player = slot.player;
    if (player !== null) {
      handlers.current.onState?.(player.state);
      if (player.audio !== null) {
        handlers.current.onAudio?.(player.audio);
      }
    }
  };

  useEffect(() => {
    const media = mediaRef.current;
    if (media === null) {
      return;
    }
    const slot = createSlot(media, false);
    activeRef.current = slot;
    videoRef.current = slot.video;
    handlers.current.onVideo?.(slot.video);
    void attachPlayer(slot).then((player) => {
      if (player !== null) {
        setReady(true);
      }
    });

    return () => {
      for (const current of [pendingRef.current, activeRef.current]) {
        if (current !== null) {
          destroySlot(current);
        }
      }
      pendingRef.current = null;
      activeRef.current = null;
      videoRef.current = null;
      handlers.current.onVideo?.(null);
      setReady(false);
    };
  }, [videoRef]);

  useEffect(() => {
    for (const slot of [activeRef.current, pendingRef.current]) {
      slot?.captions.setVisible(captions);
    }
  }, [captions]);

  useEffect(() => {
    for (const slot of [activeRef.current, pendingRef.current]) {
      slot?.captions.setSuperimposeVisible(superimpose);
    }
  }, [superimpose]);

  const { stroke, backgroundOpacity } = captionStyle;
  useEffect(() => {
    for (const slot of [activeRef.current, pendingRef.current]) {
      slot?.captions.setStyle({ stroke, backgroundOpacity });
    }
  }, [stroke, backgroundOpacity]);

  // 聞く音声を選ぶ。切り替えは、これから変換する部分から効く (設計書 12 章のデュアルモノ)。
  // 読み直すとプレイヤーが既定の音声に戻ることがあるので、読み直すたびにも選び直す。
  useEffect(() => {
    const player = activeRef.current?.player ?? null;
    if (ready && player !== null) {
      selectAudio(player, audio);
    }
  }, [ready, audio]);

  // URL が変わったら読み直す。URL がなければ、読み込みを止めて、帯域とチューナーを使わない。
  useEffect(() => {
    const active = activeRef.current;
    const player = active?.player ?? null;
    const media = mediaRef.current;
    if (!ready || active === null || player === null || media === null) {
      return;
    }
    if (url === null) {
      player.stop();

      return;
    }
    let cancelled = false;
    // 今の映像が出ていれば、流したまま裏で読み込む。出ていなければ、待たせる映像がないので、すぐ読み直す。
    if (handoverRef.current && hasPicture(active.video)) {
      const slot = createSlot(media, true);
      pendingRef.current = slot;
      void attachPlayer(slot)
        .then(async (next) => {
          if (next === null || cancelled) {
            return;
          }
          selectAudio(next, audioRef.current);
          await next.load(url);
          // 映像が出る前に、読み込みが失敗するか終わったら、切り替えずに失敗として知らせる。
          await Promise.race([firstPicture(slot.video), failureBeforePicture(next)]);
          if (!cancelled) {
            promote(slot);
            handlers.current.onLoaded?.();
          }
        })
        .catch((error: unknown) => {
          if (cancelled || pendingRef.current !== slot) {
            return;
          }
          pendingRef.current = null;
          destroySlot(slot);
          handlers.current.onError?.(error instanceof Error ? error : new Error(String(error)));
        });

      return () => {
        cancelled = true;
        // 切り替える前に URL が変わったら、裏で読み込んでいた組を捨てる。
        if (pendingRef.current === slot) {
          pendingRef.current = null;
          destroySlot(slot);
        }
      };
    }
    active.captions.reset();
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
      // 裏で読み込んで切り替えるなら、前の読み込みは、次の URL を裏で読み込んで切り替えるまで流し続ける。
      // 次の実行が裏で読み込まないとき (映像が出ていない、URL がない) は、そこで止めるか読み直すので、ここでは止めない。
      // そうでなければ、前の読み込みを止めて、サーバーに無駄な読み込みをさせない。
      if (!handoverRef.current) {
        player.stop();
      }
    };
  }, [ready, url]);

  // 再生しているのに映像が進まなければ、配信が切れたとみなして、エラーとして知らせる。利用者が止めたとき、シークしているとき、
  // 終わりまで再生したときは見張らない。読み込んで映像が出るまでも見張らない。
  useEffect(() => {
    if (!ready || url === null) {
      return;
    }
    let last: { slot: Slot; time: number; since: number } | null = null;
    let reported = false;
    const timer = setInterval(() => {
      const slot = activeRef.current;
      const video = slot?.video;
      const player = slot?.player;
      if (slot === null || video === undefined || player === undefined || player === null) {
        return;
      }
      const now = Date.now();
      const watching =
        !video.paused &&
        !video.seeking &&
        !video.ended &&
        player.state !== 'loading' &&
        player.state !== 'seeking' &&
        hasPicture(video);
      if (!watching || last?.slot !== slot || last.time !== video.currentTime) {
        last = { slot, time: video.currentTime, since: now };
        reported = false;

        return;
      }
      if (!reported && now - last.since >= stallTimeoutMs) {
        reported = true;
        handlers.current.onError?.(new Error('the stream stalled'));
      }
    }, 1000);

    return () => clearInterval(timer);
  }, [ready, url]);

  return (
    <div className={classes.frame}>
      {/* 切り抜き (#496) で拡大した映像と字幕を、枠の外に出さない。 */}
      <div className={classes.clip}>
        <div ref={mediaRef} className={classes.media} style={cropStyle(crop)} />
      </div>
      {overlay !== undefined && <div className={classes.overlay}>{overlay}</div>}
      {controls}
    </div>
  );
}

// hasPicture は、video に、今映せる映像があるかどうかを返す。
function hasPicture(video: HTMLVideoElement): boolean {
  return video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA;
}

// handoverTimeoutMs は、裏で読み込んだ映像が出るのを待つ時間の上限。
const handoverTimeoutMs = 15_000;

// failureBeforePicture は、裏で読み込んでいるプレイヤーが、失敗するか、流し終えるか、handoverTimeoutMs が過ぎたら、失敗で終わる。
// 映像が出る前に終わったら、待ち続けずに、知らせを出して間隔を延ばすほうに回す。
function failureBeforePicture(player: Mpeg2TsPlayer): Promise<never> {
  return new Promise((_, reject) => {
    const timer = setTimeout(
      () => reject(new Error('the new stream did not show a picture in time')),
      handoverTimeoutMs,
    );
    player.addEventListener('error', (event) => {
      clearTimeout(timer);
      reject(event.detail.error);
    });
    player.addEventListener('statechange', (event) => {
      if (event.detail.state === 'completed' || event.detail.state === 'error') {
        clearTimeout(timer);
        reject(new Error(`the new stream ended before a picture (${event.detail.state})`));
      }
    });
  });
}

// firstPicture は、video に最初の映像が入るのを待つ。
function firstPicture(video: HTMLVideoElement): Promise<void> {
  if (hasPicture(video)) {
    return Promise.resolve();
  }

  return new Promise((resolve) => {
    video.addEventListener('loadeddata', () => resolve(), { once: true });
  });
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
