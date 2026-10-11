import { useEffect, useRef, useState, type ReactNode, type RefObject } from 'react';
import { BMLBrowser } from 'web-bml';

import { cspNonce } from '../futon';
import type { DataBroadcastEnd, DataBroadcastRequest } from './dataBroadcastWorker';
import type { DataBroadcastSource } from './route';
import classes from './TsPlayer.module.css';

// RemoteKey は、リモコンのボタン。値は web-bml の AribKeyCode (content.ts) と同じにする。
export const RemoteKey = {
  up: 1,
  down: 2,
  left: 3,
  right: 4,
  enter: 18,
  back: 19,
  d: 20,
  blue: 21,
  red: 22,
  green: 23,
  yellow: 24,
} as const;

export type RemoteKey = (typeof RemoteKey)[keyof typeof RemoteKey];

// DataBroadcastState は、データ放送の表示の状態。
// loading は、データ放送のモジュールを受け取るまで。shown は、文書を表示したあと。
// ended は、TS を読めなくなったこと。
export type DataBroadcastState = 'off' | 'loading' | 'shown' | 'ended';

// Resolution は、データ放送の文書の大きさ (ARIB STD-B24 では、960x540 か 720x480)。
type Resolution = { width: number; height: number };

const defaultResolution: Resolution = { width: 960, height: 540 };

// useDataBroadcast は、web-bml でデータ放送を描き、映像に重ねる層を返す (設計書 7.4)。
// source が null なら、何もしない。TS は Web Worker で解き、描くのは画面のスレッドの BMLBrowser に任せる。
//
// web-bml は、渡した要素 (mediaElement) を、文書の中の映像の位置へ、ページが変わるたびに移す。そこは不透明なので、
// 映像を下に透かすことはできない。プレイヤーが作った映像の要素 (React は持たない) を mediaElement の中へ移し、
// 文書を隠すときと、表示をやめるときに、プレイヤーへ戻す。
export function useDataBroadcast(
  source: DataBroadcastSource | null,
  videoRef: RefObject<HTMLVideoElement | null>,
): { layer: ReactNode; state: DataBroadcastState; press: (key: RemoteKey) => void } {
  // layer は、文書を描く層。画質を変えるとプレイヤーごと作り直されるので、要素が変わったら描き直す。
  const [layer, setLayer] = useState<HTMLDivElement | null>(null);
  const browserRef = useRef<BMLBrowser | null>(null);
  const [state, setState] = useState<DataBroadcastState>('off');
  const [resolution, setResolution] = useState<Resolution>(defaultResolution);
  // invisible は、データ放送の文書が、表示しないよう求めていること。映像は全面に戻す。
  const [invisible, setInvisible] = useState(false);
  const [scale, setScale] = useState(1);
  const url = source?.url ?? null;
  const serviceId = source?.serviceId ?? null;

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
    const browser = new BMLBrowser({
      containerElement: container,
      mediaElement,
      videoPlaneModeEnabled: true,
      tabIndex: 0,
      styleNonce: cspNonce(),
      // 既定では <dialog> を出すので、画面を塞がないよう、ログに残すだけにする。
      showErrorMessage: (title, message) => console.warn(`data broadcast: ${title}: ${message}`),
      // 既定では、受け取ったモジュールや API の呼び出しを、すべてコンソールに出す。
      log: { level: 'warn' },
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
      if (hidden || video === null || browser.getVideoElement() === null) {
        putBack();
      } else if (restore === null) {
        restore = moveMedia(video, mediaElement);
      }
    };
    browser.addEventListener('load', (event) => {
      setResolution(event.detail.resolution);
      setState('shown');
      place();
    });
    browser.addEventListener('invisible', (event) => {
      hidden = event.detail;
      setInvisible(event.detail);
      place();
    });

    const worker = new Worker(new URL('./dataBroadcastWorker.ts', import.meta.url), {
      type: 'module',
    });
    worker.addEventListener('message', (event: MessageEvent) => {
      const message = event.data as Parameters<BMLBrowser['emitMessage']>[0] | DataBroadcastEnd;
      if (message.type === 'kotatsu:end') {
        // 文書は更新されなくなるので、層を隠し、映像をプレイヤーへ戻す。
        hidden = true;
        setInvisible(true);
        setState('ended');
        place();

        return;
      }
      browser.emitMessage(message);
    });
    worker.postMessage({ url, serviceId } satisfies DataBroadcastRequest);

    return () => {
      worker.terminate();
      putBack();
      browser.destroy();
      browserRef.current = null;
      container.remove();
      setInvisible(false);
    };
  }, [url, serviceId, layer, videoRef]);

  // 文書を、層の幅に合わせて拡大する。
  useEffect(() => {
    if (layer === null) {
      return;
    }
    const observer = new ResizeObserver(() => setScale(layer.clientWidth / resolution.width));
    observer.observe(layer);

    return () => observer.disconnect();
  }, [layer, resolution.width]);

  const press = (key: RemoteKey) => {
    const browser = browserRef.current;
    if (browser === null) {
      return;
    }
    browser.content.processKeyDown(key);
    browser.content.processKeyUp(key);
  };

  const element = (
    <div
      ref={setLayer}
      className={classes.dataBroadcast}
      hidden={url === null || invisible}
      style={{
        // 文書は、決まった大きさで描かれるので、層の大きさに合わせて拡大する。
        ['--data-broadcast-width' as string]: `${resolution.width}px`,
        ['--data-broadcast-height' as string]: `${resolution.height}px`,
        ['--data-broadcast-scale' as string]: String(scale),
      }}
    />
  );

  return { layer: element, state, press };
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
