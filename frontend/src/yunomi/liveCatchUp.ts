import { useEffect, useEffectEvent, useState, useSyncExternalStore } from 'react';

import {
  createLiveBufferController,
  defaultLiveBufferParams,
  loadLiveBufferFloor,
  prefersConservativeRate,
  saveLiveBufferFloor,
  type LiveBufferStatus,
} from './liveBuffer';
import type { VideoLag } from './useVideoLag';
import { browserStorage } from './volume';

// ライブの低遅延モード (#495、#1514、設計書 7.2、7.3)。貯める量 (目標量) を、止まり方と届き方から決め、
// 再生の速さで寄せて、放送からの遅れを小さくする。決める部分は liveBuffer.ts にあり、ここは video の要素を読んで渡す。
// 端末ごとに選び、既定では切る。

// catchUpRate は、溜まりすぎのときに速める再生の速さ。kyusu の経路でも、hls.js が追いつく速さの上限に使う。
export const catchUpRate = defaultLiveBufferParams.speedUpRate;

// liveLag は、video の、読み込んだ映像の末尾と今の再生位置との差 (秒) を返す。まだ読み込んでいなければ 0。
export function liveLag(video: Pick<HTMLVideoElement, 'buffered' | 'currentTime'>): number {
  return Math.max(bufferedTail(video) - video.currentTime, 0);
}

// bufferedTail は、読み込んだ映像の末尾。まだ読み込んでいなければ 0。
function bufferedTail(video: Pick<HTMLVideoElement, 'buffered'>): number {
  const { buffered } = video;

  return buffered.length === 0 ? 0 : buffered.end(buffered.length - 1);
}

// LiveCatchUp は、再生の情報に出す制御の様子と、保存した下限を破棄する操作。
export type LiveCatchUp = {
  // status は、制御の様子。制御していなければ null。
  status(): LiveBufferStatus | null;
  // forgetFloor は、保存した下限を破棄し、目標量を初期の最小量まで詰めて、放送の今へ戻す。
  forgetFloor(): void;
};

// useLiveCatchUp は、enabled の間、video のバッファを計測して目標量を決め、再生の速さで寄せる。
// 制御 (目標量と下限) は、画面を開いている間は、画質を変えたり、つなぎ直したりして video が変わっても引き継ぐ。
// pictureInPicture は、別のウィンドウに出しているか。出している間は、タブが隠れても見えているとみなす。
// videoLag は、映像だけの遅れの制御 (#1513)。映像だけ遅れている間は速めず、自分で飛び直して起きた詰まりは数えない。
export function useLiveCatchUp(
  video: HTMLVideoElement | null,
  enabled: boolean,
  {
    pictureInPicture,
    videoLag,
  }: { pictureInPicture: boolean; videoLag?: Pick<VideoLag, 'speedUpAllowed' | 'excusesStall'> },
): LiveCatchUp {
  const [controller] = useState(() =>
    createLiveBufferController({
      now: performance.now(),
      storedFloor: loadLiveBufferFloor(browserStorage()),
      onFloorChange: (floor) => saveLiveBufferFloor(browserStorage(), floor),
    }),
  );

  // 別のウィンドウに出したり戻したりしても、制御を作り直さない。作り直すと、追っかけをやめて飛び、詰まりもしばらく数えなくなる。
  const inPictureInPicture = useEffectEvent(() => pictureInPicture);
  const speedUpAllowed = useEffectEvent(() => videoLag?.speedUpAllowed() ?? true);
  const excusesStall = useEffectEvent(() => videoLag?.excusesStall() ?? false);
  // 出し入れしたら、前の到着時刻を忘れる。隠れたタブで戻したときに、絞られた間隔を数えないため。
  useEffect(() => {
    controller.forgetArrival();
  }, [pictureInPicture, controller]);

  useEffect(() => {
    if (video === null || !enabled) {
      return;
    }
    const params = defaultLiveBufferParams;
    const conservativeBrowser = prefersConservativeRate(
      navigator.userAgent,
      navigator.maxTouchPoints,
    );
    const visible = () =>
      document.visibilityState === 'visible' ||
      inPictureInPicture() ||
      document.pictureInPictureElement === video;
    // counting は、届く間隔を数えるか。隠れたタブは、届き方も描き方も絞られ、間隔が長く見えるので数えない。
    const counting = () => !video.paused && !video.seeking && visible();
    // 新しい video は、MediaSource を作り直したものなので、直後の詰まりは数えない。
    controller.selfInflicted(performance.now());
    let lastTail = bufferedTail(video);

    // observe は、末尾が伸びていればチャンクが届いたとみなして通知し、今のバッファを記録して返す。
    const observe = (now: number): number => {
      const tail = bufferedTail(video);
      const buffer = Math.max(tail - video.currentTime, 0);
      if (tail > lastTail + 1e-3) {
        if (counting()) {
          // 届く直前のバッファは、前の末尾と今の再生位置との差。
          controller.arrive(
            now,
            lastTail > 0 ? Math.max(lastTail - video.currentTime, 0) : null,
            tail,
          );
        } else {
          controller.forgetArrival();
        }
      }
      lastTail = tail;
      if (counting()) {
        controller.sample(now, buffer);
      }

      return buffer;
    };

    const timer = window.setInterval(() => {
      const now = performance.now();
      const buffer = observe(now);
      // 止めている間は、無事が続いたとはみなさず、決め直さない。
      if (video.paused || video.seeking) {
        return;
      }
      controller.tick(now);
      const decision = controller.decide(now, {
        buffer,
        // タブが隠れている間は、速めたまま戻らないおそれがあるので、遅くせず、変える回数を減らす寄せ方にする。
        conservative: conservativeBrowser || !visible(),
        speedUpAllowed: speedUpAllowed(),
      });
      if (decision.jump) {
        video.currentTime = Math.max(bufferedTail(video) - controller.target, video.currentTime);
        controller.selfInflicted(now);
        video.playbackRate = 1;

        return;
      }
      if (video.playbackRate !== decision.rate) {
        video.playbackRate = decision.rate;
      }
    }, params.tickMs);

    // 映ったフレームごとにも、バッファを記録する。束が届く直前の低い所を捉えるため。
    let frameRequest: number | null = null;
    const onFrame = () => {
      observe(performance.now());
      frameRequest = video.requestVideoFrameCallback(onFrame);
    };
    if ('requestVideoFrameCallback' in video) {
      frameRequest = video.requestVideoFrameCallback(onFrame);
    }

    // 止まったら、待たずに目標量を決め直す。一時停止とシークで止まったのは数えない。
    // 映像だけの遅れを直そうと自分で飛び直して起きた詰まりも数えない。数えると、遅れを直すたびに目標量が増える。
    const onWaiting = () => {
      if (!video.paused && !video.seeking && !excusesStall()) {
        controller.stall(performance.now());
      }
    };
    const onPause = () => {
      if (!video.ended) {
        controller.pause();
      }
    };
    const onSeeking = () => controller.forgetArrival();
    const onVisibility = () => controller.forgetArrival();
    video.addEventListener('waiting', onWaiting);
    video.addEventListener('pause', onPause);
    video.addEventListener('seeking', onSeeking);
    document.addEventListener('visibilitychange', onVisibility);

    return () => {
      window.clearInterval(timer);
      if (frameRequest !== null) {
        video.cancelVideoFrameCallback(frameRequest);
      }
      video.removeEventListener('waiting', onWaiting);
      video.removeEventListener('pause', onPause);
      video.removeEventListener('seeking', onSeeking);
      document.removeEventListener('visibilitychange', onVisibility);
      video.playbackRate = 1;
    };
  }, [video, enabled, controller]);

  return {
    status: () => (video !== null && enabled ? controller.status(performance.now()) : null),
    forgetFloor: () => {
      const now = performance.now();
      controller.forgetFloor(now);
      if (video !== null && enabled) {
        // 前の経路の値のまま遅れているのを解くため、放送の今へ戻す。
        video.currentTime = Math.max(bufferedTail(video) - controller.target, video.currentTime);
        controller.selfInflicted(now);
        video.playbackRate = 1;
      }
    },
  };
}

const storageKey = 'kotatsu.player.lowLatency';
// changeEvent は、同じタブの中で変えたことを通知するイベント。storage イベントは、ほかのタブにしか届かない。
const changeEvent = 'kotatsu:low-latency';

// loadLowLatency は、低遅延モードの設定を読む。読めなければ、既定の false を返す。
export function loadLowLatency(storage: Pick<Storage, 'getItem'> | null): boolean {
  try {
    return storage?.getItem(storageKey) === 'true';
  } catch {
    return false;
  }
}

function subscribe(onChange: () => void): () => void {
  window.addEventListener('storage', onChange);
  window.addEventListener(changeEvent, onChange);

  return () => {
    window.removeEventListener('storage', onChange);
    window.removeEventListener(changeEvent, onChange);
  };
}

// useLowLatency は、低遅延モードを使うかと、それを変える関数を返す。ほかのタブで変えても追う。
export function useLowLatency(): [boolean, (enabled: boolean) => void] {
  const enabled = useSyncExternalStore(subscribe, () => loadLowLatency(browserStorage()));

  return [
    enabled,
    (next) => {
      try {
        browserStorage()?.setItem(storageKey, String(next));
      } catch {
        // 書けなければ、次に開いたときは既定に戻るだけ。
      }
      window.dispatchEvent(new Event(changeEvent));
    },
  ];
}
