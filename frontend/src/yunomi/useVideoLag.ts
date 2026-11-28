import { useEffect, useEffectEvent, useRef } from 'react';

import { defaultLiveBufferParams } from './liveBuffer';
import {
  createVideoLagController,
  defaultVideoLagParams,
  lagMeasurable,
  lagOf,
  type VideoLagController,
  type VideoLagStatus,
} from './videoLag';

// VideoLag は、映像だけの遅れの制御を、ほかの制御 (#1514 の速め) と再生の情報から読む口。
export type VideoLag = {
  // speedUpAllowed は、追いつくための速めをしてよいか。映像だけ遅れている間は速めない。速めるとデコードがさらに重くなる。
  speedUpAllowed(): boolean;
  // excusesStall は、今の詰まりが、自分で飛び直したことで起きたものか。そうなら、目標量 (#1514) の決め直しに数えない。
  excusesStall(): boolean;
  // status は、再生の情報に出す様子。video が無ければ null。
  status(): VideoLagStatus | null;
};

// qualityIntervalMs は、破棄したコマを読み、重さを判定する間隔。
const qualityIntervalMs = 1_000;

// countedRate は、重さの判定に数える再生の速さの上限。低遅延モードで寄せる速さ (#1514) までは数え、倍速や早送りは数えない。
// 速いと、遅くない端末でもコマを破棄する。
const countedRate = defaultLiveBufferParams.speedUpRate;

// aheadOf は、今の位置から先に読み込んである量 (秒)。
function aheadOf(video: HTMLVideoElement): number {
  const { buffered, currentTime } = video;
  for (let i = 0; i < buffered.length; i++) {
    if (buffered.start(i) <= currentTime && currentTime <= buffered.end(i)) {
      return buffered.end(i) - currentTime;
    }
  }

  return 0;
}

// useVideoLag は、映ったコマごとに映像だけの遅れを計測し、飛び直す遅れを超えたら currentTime を少し先へ進めて飛び直す (#1513、設計書 7.2)。
// 飛ぶとデコーダは積み残しを破棄して出し直す。遅れを測れない端末 (Windows) では、詰まりのたびに 1 回飛び直す。
// onHeavy を渡すと、その端末にはその段階が重いと判定したときに呼ぶ。呼んだら数えたものを破棄し、新しい証拠が溜まるまで呼ばない。
// video が変わったら (画質の切り替え、つなぎ直し、MediaSource の作り直し)、数えたものを破棄する。
// picturesOnCanvas が true を返す間 (コマを canvas に描き、<video> は音と時計だけの読み込み、#2287) は飛び直さない。
// 映像と音の合わせ方は、その読み込みのプレイヤーが受け持つ。
export function useVideoLag(
  video: HTMLVideoElement | null,
  { onHeavy, picturesOnCanvas }: { onHeavy?: () => void; picturesOnCanvas?: () => boolean } = {},
): VideoLag {
  const controllerRef = useRef<VideoLagController | null>(null);
  const heavy = useEffectEvent(() => onHeavy?.());
  // 段階を下げてよいかが再生の途中で変わっても (追っかけ再生の録画を終えたなど)、制御を作り直さない。
  const judging = useEffectEvent(() => onHeavy !== undefined);
  const onCanvas = useEffectEvent(() => picturesOnCanvas?.() === true);

  useEffect(() => {
    if (video === null) {
      return;
    }
    const controller = createVideoLagController({ measurable: lagMeasurable(navigator) });
    controllerRef.current = controller;
    let stopped = false;

    // jump は、currentTime を飛び直しの幅だけ先へ進める。同じ値を代入し直しても飛んだことにならない。
    const jump = (now: number, cause: 'lag' | 'stall') => {
      if (onCanvas()) {
        return;
      }
      video.currentTime += defaultVideoLagParams.jumpWidth;
      controller.jumped(now, cause, video.playbackRate <= countedRate);
    };

    let frameRequest: number | null = null;
    const onFrame = (_now: number, metadata: VideoFrameCallbackMetadata) => {
      if (stopped) {
        return;
      }
      frameRequest = video.requestVideoFrameCallback(onFrame);
      // 一時停止中と seek 中は飛ばない。
      if (video.paused || video.seeking) {
        return;
      }
      const now = performance.now();
      if (
        controller.frame(now, {
          lag: lagOf(video.currentTime, metadata.mediaTime),
          ahead: aheadOf(video),
        })
      ) {
        jump(now, 'lag');
      }
    };
    if (typeof video.requestVideoFrameCallback === 'function') {
      frameRequest = video.requestVideoFrameCallback(onFrame);
    }

    const onSeeking = () => controller.seeking();
    const onWaiting = () => {
      if (!video.paused && !video.seeking) {
        controller.stall();
      }
    };
    const onPlaying = () => {
      const now = performance.now();
      if (!video.paused && !video.seeking && controller.resumed(now, aheadOf(video))) {
        jump(now, 'stall');
      }
    };
    // 隠れていたタブが見えたら、最初の 1 コマは隠れる前の時刻を持っていることがあるので計測しない。
    const onVisibility = () => controller.seeking();
    video.addEventListener('seeking', onSeeking);
    video.addEventListener('waiting', onWaiting);
    video.addEventListener('playing', onPlaying);
    document.addEventListener('visibilitychange', onVisibility);

    // 破棄したコマは、倍速や早送りでなく再生していて、タブが見えている間だけ数える。
    const timer = window.setInterval(() => {
      const now = performance.now();
      const quality = video.getVideoPlaybackQuality?.();
      const counting =
        quality !== undefined &&
        !video.paused &&
        !video.seeking &&
        video.playbackRate <= countedRate &&
        document.visibilityState === 'visible';
      if (counting) {
        controller.quality(now, quality.totalVideoFrames, quality.droppedVideoFrames);
      } else {
        controller.forgetQuality();
      }
      if (judging() && controller.heavy(now)) {
        controller.reset();
        heavy();
      }
    }, qualityIntervalMs);

    return () => {
      stopped = true;
      if (frameRequest !== null) {
        video.cancelVideoFrameCallback(frameRequest);
      }
      window.clearInterval(timer);
      video.removeEventListener('seeking', onSeeking);
      video.removeEventListener('waiting', onWaiting);
      video.removeEventListener('playing', onPlaying);
      document.removeEventListener('visibilitychange', onVisibility);
      if (controllerRef.current === controller) {
        controllerRef.current = null;
      }
    };
  }, [video]);

  return {
    speedUpAllowed: () => controllerRef.current?.speedUpAllowed(performance.now()) ?? true,
    excusesStall: () => controllerRef.current?.excusesStall(performance.now()) ?? false,
    status: () => controllerRef.current?.status(performance.now()) ?? null,
  };
}
