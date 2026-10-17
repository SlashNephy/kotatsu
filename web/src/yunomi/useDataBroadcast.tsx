import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';
import { BMLBrowser } from 'web-bml';

import { cspNonce } from '../futon';
import {
  bmlStoragePrefix,
  loadDataBroadcastZipcode,
  writeZipcodeNVRAM,
} from './dataBroadcastZipcode';
import { browserStorage } from './volume';
import { broadcastFontUrl } from './broadcastFont';
import { createDataBroadcastAudio, type DataBroadcastAudio } from './dataBroadcastAudio';
import type { DataBroadcastEnd, DataBroadcastRequest } from './dataBroadcastWorker';
import { deinterlacerCanvas } from './deinterlacer';
import { reconnectDelay } from './reconnect';
import type { DataBroadcastSource } from './route';
import classes from './TsPlayer.module.css';

// RemoteKey は、リモコンのボタン。値は web-bml の AribKeyCode (content.ts) と同じにする。
export const RemoteKey = {
  up: 1,
  down: 2,
  left: 3,
  right: 4,
  digit0: 5,
  digit1: 6,
  digit2: 7,
  digit3: 8,
  digit4: 9,
  digit5: 10,
  digit6: 11,
  digit7: 12,
  digit8: 13,
  digit9: 14,
  digit10: 15,
  digit11: 16,
  digit12: 17,
  enter: 18,
  back: 19,
  d: 20,
  blue: 21,
  red: 22,
  green: 23,
  yellow: 24,
} as const;

export type RemoteKey = (typeof RemoteKey)[keyof typeof RemoteKey];

// KeyGroup は、文書が使うと宣言するキーのグループ (ARIB STD-B24 の used-key-list)。
export type KeyGroup = 'basic' | 'data-button' | 'numeric-tuning';

// defaultKeyGroups は、文書が使うキーを宣言していないときに使えるとみなすグループ。
const defaultKeyGroups: ReadonlySet<KeyGroup> = new Set(['basic', 'data-button']);

// usedKeyGroups は、web-bml が知らせた used-key-list から、リモコンのボタンを押せるグループを返す (#441)。
// 宣言がなければ、basic と data-button とみなす。
export function usedKeyGroups(list: ReadonlySet<string>): ReadonlySet<KeyGroup> {
  const groups = new Set<KeyGroup>();
  for (const group of list) {
    if (group === 'basic' || group === 'data-button' || group === 'numeric-tuning') {
      groups.add(group);
    }
  }

  return list.size === 0 ? defaultKeyGroups : groups;
}

// keyGroup は、リモコンのボタンが属するグループを返す。
export function keyGroup(key: RemoteKey): KeyGroup {
  if (key >= RemoteKey.digit0 && key <= RemoteKey.digit12) {
    return 'numeric-tuning';
  }
  if (key >= RemoteKey.blue && key <= RemoteKey.yellow) {
    return 'data-button';
  }

  return 'basic';
}

// DataBroadcastState は、データ放送の表示の状態。
// loading は、データ放送のモジュールを受け取るまで。shown は、文書を表示したあと。
// ended は、TS を読めなくなったこと。
export type DataBroadcastState = 'off' | 'loading' | 'shown' | 'ended';

// Resolution は、データ放送の文書の大きさ (ARIB STD-B24 では、960x540 か 720x480)。
type Resolution = { width: number; height: number };

const defaultResolution: Resolution = { width: 960, height: 540 };

// Fit は、文書を層に収める拡大率と、中央に寄せるための左上の位置 (px)。
export type Fit = { scale: number; x: number; y: number };

// fitDocument は、文書を、縦横の比を保って層に収める拡大率と位置を返す (#444)。
// 縦と横の小さいほうの比率で拡大し、余りは中央に寄せる。層の比が 16:9 でない (720x480 の文書や、16:9 でない画面の全画面) ときも、はみ出さない。
export function fitDocument(layer: { width: number; height: number }, resolution: Resolution): Fit {
  const scale = Math.min(layer.width / resolution.width, layer.height / resolution.height);
  if (!Number.isFinite(scale) || scale <= 0) {
    return { scale: 1, x: 0, y: 0 };
  }

  return {
    scale,
    x: (layer.width - resolution.width * scale) / 2,
    y: (layer.height - resolution.height * scale) / 2,
  };
}

// enterIntervalMs と maxEnterTries は、データ放送の画面に入るために、d ボタンを押し直す間隔と回数 (20 秒まで)。
const enterIntervalMs = 200;
const maxEnterTries = 100;

// useDataBroadcast は、web-bml でデータ放送を描き、映像に重ねる層を返す (設計書 7.4)。
// source が null なら、何もしない。TS は Web Worker で解き、描くのは画面のスレッドの BMLBrowser に任せる。
//
// web-bml は、渡した要素 (mediaElement) を、文書の中の映像の位置へ、ページが変わるたびに移す。そこは不透明なので、
// 映像を下に透かすことはできない。プレイヤーが作った映像の要素 (React は持たない) を mediaElement の中へ移し、
// 文書を隠すときと、表示をやめるときに、プレイヤーへ戻す。
//
// shown が false の間も、source があれば裏で受け取っておく。文書のスクリプトはカルーセルで届くので、受け取り始めてから
// d ボタンを受け取れるようになるまで、局によっては数秒かかる。出すときには、その待ちを済ませておく。
// DataBroadcastHandlers は、文書が選局したり、音声を切り替えたりしようとしたときに呼ぶもの (#440)。
// 文書が呼ぶ API は、その場で受け付けたかどうかを返すので、どちらも真偽値を返す。
export type DataBroadcastHandlers = {
  // onTune は、文書が (original_network_id, service_id) のサービスへ選局しようとしたときに呼ぶ。移れなければ false を返す。
  onTune?: (networkId: number, serviceId: number) => boolean;
  // onSelectAudio は、文書が component_tag の音声 (デュアルモノなら channelId の音声) を選ぼうとしたときに呼ぶ。
  onSelectAudio?: (componentTag: number, channelId?: number) => boolean;
};

export function useDataBroadcast(
  source: DataBroadcastSource | null,
  videoRef: RefObject<HTMLVideoElement | null>,
  shown: boolean,
  handlers: DataBroadcastHandlers = {},
): {
  layer: ReactNode;
  state: DataBroadcastState;
  // receiving は、出すことになったのに、まだデータ放送の画面を出せていないこと (受け取っているか、d ボタンで入っている途中)。
  receiving: boolean;
  // keyGroups は、いまの文書が使うキーのグループ。
  keyGroups: ReadonlySet<KeyGroup>;
  press: (key: RemoteKey) => void;
} {
  // handlersRef は、最新の handlers。BMLBrowser は作り直さないので、呼ぶときに今のものを読む。
  const handlersRef = useRef(handlers);
  useEffect(() => {
    handlersRef.current = handlers;
  });
  // layer は、文書を描く層。画質を変えるとプレイヤーごと作り直されるので、要素が変わったら描き直す。
  const [layer, setLayer] = useState<HTMLDivElement | null>(null);
  const browserRef = useRef<BMLBrowser | null>(null);
  // audioRef は、データ放送の効果音の出口。ユーザーの操作の中で、止まっている AudioContext を動かす (#439)。
  const audioRef = useRef<DataBroadcastAudio | null>(null);
  // stopEnteringRef は、データ放送の画面に入るために d ボタンを押し直すのを、やめる。
  const stopEnteringRef = useRef<() => void>(() => {});
  // showRef と hideRef は、出すかどうかが変わったときに、受け取っている文書へ伝える。
  const showRef = useRef<() => void>(() => {});
  const hideRef = useRef<() => void>(() => {});
  const shownRef = useRef(shown);
  shownRef.current = shown;
  const [state, setState] = useState<DataBroadcastState>('off');
  // pressing は、データ放送の画面に入るために、d ボタンを押し直している間。
  const [pressing, setPressing] = useState(false);
  const [resolution, setResolution] = useState<Resolution>(defaultResolution);
  // invisible は、データ放送の文書が、表示しないよう求めていること。映像は全面に戻す。
  const [invisible, setInvisible] = useState(false);
  const [keyGroups, setKeyGroups] = useState<ReadonlySet<KeyGroup>>(defaultKeyGroups);
  const [fit, setFit] = useState<Fit>({ scale: 1, x: 0, y: 0 });
  const url = source?.url ?? null;
  const serviceId = source?.serviceId ?? null;
  // generation は、データ放送の接続だけが切れたときに、つなぎ直すたびに増やす (#446)。
  const [generation, setGeneration] = useState(0);
  // attempt は、続けてつなぎ直した回数。文書を読めたら 0 に戻す。
  const attempt = useRef(0);
  // 映像をつなぎ直して受け取る先が変わったら、データ放送も最初の間隔から数え直す。
  useEffect(() => {
    attempt.current = 0;
  }, [url, serviceId]);

  useEffect(() => {
    if (url === null || serviceId === null || layer === null) {
      setState('off');

      return;
    }
    setState('loading');
    // BMLBrowser は、渡した要素に shadow root を付ける。React が中身を持たない要素を作って渡す。
    const container = document.createElement('div');
    layer.append(container);
    const mediaElement = document.createElement('div');
    mediaElement.style.width = '100%';
    mediaElement.style.height = '100%';
    // 字幕の canvas は、aribb24.js が絶対位置で映像に重ねるので、その基準にする。
    mediaElement.style.position = 'relative';
    // 受信機の郵便番号を、文書が読む NVRAM に書いておく (#442)。
    writeZipcodeNVRAM(browserStorage(), loadDataBroadcastZipcode(browserStorage()));
    // 効果音は、プレイヤーの音量とミュートに合わせて鳴らす (#439)。
    const audio = createDataBroadcastAudio(() => videoRef.current);
    audioRef.current = audio;
    const browser = new BMLBrowser({
      ...(audio !== null ? { audioNodeProvider: audio } : {}),
      // web-bml が localStorage に書くキーを、Kotatsu 自身のキーと分ける。
      storagePrefix: bmlStoragePrefix,
      containerElement: container,
      mediaElement,
      videoPlaneModeEnabled: true,
      tabIndex: 0,
      styleNonce: cspNonce(),
      // 既定では <dialog> を出すので、画面を塞がないよう、ログに残すだけにする。
      showErrorMessage: (title, message) => console.warn(`data broadcast: ${title}: ${message}`),
      // 既定では、受け取ったモジュールや API の呼び出しを、すべてコンソールに出す。
      log: { level: 'warn' },
      // 文書は受信機の字幅を前提に作られているので、OS のフォントで描くと、はみ出したり折り返したりする (#443)。
      // 字幕と同じ等幅の丸ゴシックで描く。角ゴシックの字形は同梱しないので、角ゴシックも同じフォントで描き、字幅をそろえる。
      // FontFace は、文書が使うまで読み込まないので、データ放送を出すときにだけ読む。
      fonts: {
        roundGothic: { source: `url(${broadcastFontUrl})` },
        squareGothic: { source: `url(${broadcastFontUrl})` },
      },
      // 文書からの選局と音声の切り替えを、視聴の画面に任せる (#440)。transport_stream_id とコンポーネントは使わない。
      epg: {
        tune: (networkId, _transportStreamId, serviceId) =>
          handlersRef.current.onTune?.(networkId, serviceId) ?? false,
        tuneToComponent: (networkId, _transportStreamId, serviceId) =>
          handlersRef.current.onTune?.(networkId, serviceId) ?? false,
      },
      setMainAudioStreamCallback: (componentId, channelId) =>
        handlersRef.current.onSelectAudio?.(componentId, channelId) ?? false,
    });
    browserRef.current = browser;
    // restore は、データ放送の中へ移した映像を、プレイヤーへ戻す。移していなければ null。
    let restore: (() => void) | null = null;
    const putBack = () => {
      restore?.();
      restore = null;
    };
    // hidden は、文書が隠れているか、TS を読めなくなって層を隠したこと。
    let hidden = false;
    // place は、映像をデータ放送の中へ移すか、プレイヤーへ戻す。どのイベントでも、同じ条件で決める。
    // 層を隠している間と、映像の位置がないページでは、mediaElement が見えないか文書の外に出るので、プレイヤーへ戻す。
    const place = () => {
      const video = videoRef.current;
      if (!shownRef.current || hidden || video === null || browser.getVideoElement() === null) {
        putBack();
      } else if (restore === null) {
        restore = moveMedia(video, mediaElement);
      }
    };
    // データ放送を入れたら、リモコンの d ボタンを押したときと同じく、データ放送の画面に入る。
    // web-bml は、load を知らせてから文書のスクリプトを走らせ、d ボタンを受け取る仕組みを最後に登録する。登録の前に押しても
    // 何も起きないので、文書が見えるようになる (d ボタンが効く) か、利用者がリモコンを使うまで、間を空けて押し直す。
    // 見えない文書は、受け取り直すと同じ文書をもう一度読む。読み直したあとに d ボタンを受け取れるようになることが多いので、
    // load では止めずに押し続ける。
    let entering: 'idle' | 'pressing' | 'done' = 'idle';
    let tries = 0;
    let enterTimer: ReturnType<typeof setTimeout> | null = null;
    const clearEnterTimer = () => {
      if (enterTimer !== null) {
        clearTimeout(enterTimer);
        enterTimer = null;
      }
    };
    const stopEntering = () => {
      clearEnterTimer();
      entering = 'done';
      setPressing(false);
    };
    stopEnteringRef.current = stopEntering;
    const pressLater = (delay: number) => {
      clearEnterTimer();
      entering = 'pressing';
      setPressing(true);
      enterTimer = setTimeout(() => {
        enterTimer = null;
        if (tries >= maxEnterTries) {
          stopEntering();

          return;
        }
        tries += 1;
        browser.content.processKeyDown(RemoteKey.d);
        browser.content.processKeyUp(RemoteKey.d);
        pressLater(enterIntervalMs);
      }, delay);
    };
    // loaded は、最初の文書を読んだかどうか。読む前に出すことになったら、読んだところで入る。
    let loaded = false;
    // enter は、出すことになっていて、まだ入っていなければ、d ボタンを押し始める。
    // 文書が見えるかは、load の直後の invisible で分かる。見える文書なら、最初に押す前に止まる。
    const enter = () => {
      if (shownRef.current && loaded && entering === 'idle') {
        pressLater(0);
      }
    };
    showRef.current = () => {
      // 出すのは、d ボタンなどのユーザーの操作なので、ここで効果音を鳴らせるようにする。
      audio?.resume();
      place();
      enter();
    };
    // 出すのをやめたら、受け取りは続けたまま、受信機で d ボタンを押して消すのと同じく、最初の見えない文書へ戻す。
    // 受け取り直すと、次に出すときに、また d ボタンを受け取れるまで待つことになる。
    hideRef.current = () => {
      stopEntering();
      if (loaded && !hidden) {
        browser.content.processKeyDown(RemoteKey.d);
        browser.content.processKeyUp(RemoteKey.d);
      }
      entering = 'idle';
      tries = 0;
      place();
    };
    browser.addEventListener('load', (event) => {
      attempt.current = 0;
      setResolution(event.detail.resolution);
      setState('shown');
      loaded = true;
      place();
      enter();
    });
    browser.addEventListener('usedkeylistchanged', (event) => {
      setKeyGroups(usedKeyGroups(event.detail.usedKeyList));
    });
    browser.addEventListener('invisible', (event) => {
      hidden = event.detail;
      if (!hidden) {
        stopEntering();
      }
      setInvisible(event.detail);
      place();
    });

    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    const worker = new Worker(new URL('./dataBroadcastWorker.ts', import.meta.url), {
      type: 'module',
    });
    worker.addEventListener('message', (event: MessageEvent) => {
      const message = event.data as Parameters<BMLBrowser['emitMessage']>[0] | DataBroadcastEnd;
      if (message.type === 'kotatsu:end') {
        // 文書は更新されなくなるので、d ボタンを押し直すのをやめ、層を隠し、映像をプレイヤーへ戻す。
        stopEntering();
        hidden = true;
        setInvisible(true);
        setState('ended');
        place();
        // 映像は流れているのに、データ放送の接続だけが切れたら、映像と同じ間隔でつなぎ直す。
        // 4xx は、つなぎ直しても同じなので、やめる。つなぎ直したら、出していれば、文書を読んだところでまた d ボタンで入る。
        if (message.status === undefined || message.status < 400 || message.status >= 500) {
          retryTimer = setTimeout(
            () => {
              attempt.current += 1;
              setGeneration((n) => n + 1);
            },
            reconnectDelay(attempt.current) * 1000,
          );
        }

        return;
      }
      browser.emitMessage(message);
    });
    worker.postMessage({ url, serviceId } satisfies DataBroadcastRequest);

    return () => {
      if (retryTimer !== null) {
        clearTimeout(retryTimer);
      }
      stopEntering();
      // 片付けた文書に、出す・消す・d ボタンの操作が届かないようにする。
      showRef.current = () => {};
      hideRef.current = () => {};
      stopEnteringRef.current = () => {};
      worker.terminate();
      putBack();
      browser.destroy();
      browserRef.current = null;
      audio?.close();
      audioRef.current = null;
      container.remove();
      setInvisible(false);
      setKeyGroups(defaultKeyGroups);
    };
  }, [url, serviceId, layer, videoRef, generation]);

  // 出すことになったら、データ放送の画面に入る。出すのをやめたら、最初の文書へ戻す。
  // 最初の描画では、まだ文書がないので、何もしない。
  const firstRun = useRef(true);
  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false;

      return;
    }
    if (shown) {
      showRef.current();
    } else {
      hideRef.current();
    }
  }, [shown]);

  // 文書を、層に収まるよう拡大する。
  const { width, height } = resolution;
  useEffect(() => {
    if (layer === null) {
      return;
    }
    const observer = new ResizeObserver(() =>
      setFit(
        fitDocument({ width: layer.clientWidth, height: layer.clientHeight }, { width, height }),
      ),
    );
    observer.observe(layer);

    return () => observer.disconnect();
  }, [layer, width, height]);

  const press = (key: RemoteKey) => {
    // 利用者がリモコンを使ったら、d ボタンを押し直すのをやめる。
    stopEnteringRef.current();
    const browser = browserRef.current;
    if (browser === null) {
      return;
    }
    audioRef.current?.resume();
    browser.content.processKeyDown(key);
    browser.content.processKeyUp(key);
  };

  const element = (
    <div
      ref={setLayer}
      className={classes.dataBroadcast}
      hidden={url === null || !shown || invisible}
      style={{
        // 文書は、決まった大きさで描かれるので、層の大きさに合わせて拡大する。
        ['--data-broadcast-width' as string]: `${resolution.width}px`,
        ['--data-broadcast-height' as string]: `${resolution.height}px`,
        ['--data-broadcast-scale' as string]: String(fit.scale),
        ['--data-broadcast-x' as string]: `${fit.x}px`,
        ['--data-broadcast-y' as string]: `${fit.y}px`,
      }}
    />
  );

  const receiving = shown && (state === 'loading' || (state === 'shown' && pressing));

  return { layer: element, state, receiving, keyGroups, press };
}

// moveMedia は、video を含むプレイヤーの映像の要素 (字幕の canvas などを含む) を target へ移し、戻す関数を返す。
// target は web-bml の閉じた shadow root の中にあり、外の CSS が届かないので、大きさをインラインで整える。
export function moveMedia(video: HTMLVideoElement, target: HTMLElement): () => void {
  const media = video.closest<HTMLElement>(`.${classes.media}`) ?? video.parentElement;
  if (media === null) {
    return () => {};
  }
  const nodes = Array.from(media.childNodes);
  const elements = nodes.flatMap((node) =>
    node instanceof HTMLElement ? [node, ...node.querySelectorAll<HTMLElement>('*')] : [],
  );
  // 戻すときに元に戻すので、移す前のインラインのスタイルを覚える。style 属性は CSP に止められるので、CSSOM で読み書きする。
  const saved = elements.map((element) => [element, element.style.cssText] as const);
  for (const element of elements) {
    // yadif の canvas は、yadif が video に合わせて置き直す。大きさを変えると、位置だけが元のままずれ、映像が二重に見える。
    if (element.dataset[deinterlacerCanvas] !== undefined) {
      continue;
    }
    element.style.display = 'block';
    element.style.width = '100%';
    element.style.height = '100%';
    if (element instanceof HTMLVideoElement || element instanceof HTMLCanvasElement) {
      element.style.objectFit = 'contain';
    }
  }
  target.replaceChildren(...nodes);

  return () => {
    media.append(...nodes);
    for (const [element, cssText] of saved) {
      element.style.cssText = cssText;
    }
  };
}
