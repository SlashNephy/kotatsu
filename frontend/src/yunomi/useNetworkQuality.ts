import { useEffect, useEffectEvent, useRef, useState } from 'react';

import { measureProbe } from './networkProbe';
import {
  audioStep,
  createNetworkController,
  defaultNetworkParams,
  type NetworkMode,
  type NetworkStatus,
  type Step,
  type StepReason,
} from './networkQuality';
import { ReceiveMeter } from './receiveRate';
import type { HlsSegment, TsPlayerReading } from './streamMeter';
import type { VideoLag } from './useVideoLag';

// NetworkPlayback は、usePlaybackRoute のうち、回線のために段を動かすのに使う部分。
export type NetworkPlayback = {
  current: string | null;
  automatic: boolean;
  networkLadder: Step[];
  setNetworkStep: (name: string) => void;
};

// NetworkStepEvent は、回線のために段を動かしたこと。
export type NetworkStepEvent = { to: string; direction: 'down' | 'up'; reason: StepReason };

export type NetworkQualityOptions = {
  // mode は、上げ方。ライブは probe、録画と追っかけは rate。
  mode: NetworkMode;
  // convert は、ブラウザ内で変換する経路か。録画の再生では、プレイヤーが先に読みすぎないよう読むのを絞るので、その間は速さを測らない。
  convert: boolean;
  // topBitrate は、最高画質 (オリジナル) のビットレート (bps)。録画の平均のビットレートなど。分からなければ null。
  topBitrate: number | null;
  // videoLag は、映像だけの遅れの制御。自分で飛び直して起きた詰まりを、回線の詰まりとして数えない。
  videoLag?: Pick<VideoLag, 'excusesStall'>;
  // pictureInPicture は、別のウィンドウに出しているか。出している間は、タブが隠れても見えているとみなす。
  // 見えていない間は、コマごとのコールバックが止まるので判定しない。
  pictureInPicture: boolean;
  onStep: (event: NetworkStepEvent) => void;
  // onShortManual は、手で選んだ画質で回線が足りないとみなしたときに呼ぶ。
  onShortManual: () => void;
};

// NetworkQuality は、再生の情報に出す様子と、届いた量を受け取る口。
export type NetworkQuality = {
  status(): NetworkStatus | null;
  // onReading と onSegment は、プレイヤーから届いた量を受け取る。StreamMeter と同じものを渡す。
  onReading: (reading: TsPlayerReading) => void;
  onSegment: (segment: HlsSegment) => void;
};

// haveFutureData は、HTMLMediaElement.HAVE_FUTURE_DATA。
const haveFutureData = 3;
// selfLimitAhead は、変換の経路の録画の再生で、プレイヤーが読むのを絞っているとみなす先の量 (秒)。
// 変換のプレイヤーは、再生位置より 8 秒先まで変換したら読むのを止める (maxAheadSeconds)。
const selfLimitAhead = 6;

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

// useNetworkQuality は、回線の側の兆しを刻みごとに見て、画質の「自動」の段を下げ、余裕を確かめてから上げる (#1513、設計書 7.1、7.5)。
// 決める部分は networkQuality.ts にあり、ここは video の要素とプレイヤーから測って渡し、決めたとおりに段を動かし、試しのデータを取る。
// 制御は、画面を開いている間は、段を替えて video が変わっても引き継ぐ。
export function useNetworkQuality(
  video: HTMLVideoElement | null,
  playback: NetworkPlayback,
  options: NetworkQualityOptions,
): NetworkQuality {
  const [controller] = useState(() => createNetworkController({ now: performance.now() }));
  const [meter] = useState(() => new ReceiveMeter());
  // lastLoad は、変換の経路で最後に読んだ読み込みの番号と、それまでに読んだバイト数。
  const lastReading = useRef<{ load: number; bytes: number } | null>(null);
  const probeAbort = useRef<AbortController | null>(null);

  const read = useEffectEvent(() => ({
    current: playback.current,
    automatic: playback.automatic,
    ladder: playback.networkLadder,
    mode: options.mode,
    convert: options.convert,
    topBitrate: options.topBitrate,
    pictureInPicture: options.pictureInPicture,
  }));
  const excused = useEffectEvent(() => options.videoLag?.excusesStall() ?? false);
  const applyStep = useEffectEvent((to: string) => playback.setNetworkStep(to));
  const stepped = useEffectEvent((event: NetworkStepEvent) => options.onStep(event));
  const shortManual = useEffectEvent(() => options.onShortManual());

  // 経路の種類 (ライブと追っかけ) が変わったら、届いた量を数え直す。
  useEffect(() => {
    meter.reset();
    lastReading.current = null;
  }, [meter, options.mode, playback.current]);

  useEffect(() => {
    if (video === null) {
      return;
    }
    let waiting = false;
    let lastFrameAt = performance.now();
    // awaiting は、段が変わって、最初のコマ (音声だけなら再生の始まり) を待っている段の名前。
    let awaiting: string | null = null;
    let seenCurrent = read().current;
    // started は、この video で (段が変わったら、その段で) 再生が始まったか。始まるまでは判定しない。
    // 読み込みに時間がかかっても、始まるまでの詰まりを「足りない」と取り違えないよう、切り替え直後の無視時間は始まってから数える。
    let started = false;
    const start = (now: number) => {
      if (!started) {
        started = true;
        controller.restart(now);
      }
    };
    let stopped = false;
    let manualNoticeAt = -Infinity;

    const onWaiting = () => {
      if (!video.paused && !video.seeking && !excused()) {
        waiting = true;
      }
    };
    const onPlaying = () => {
      waiting = false;
      start(performance.now());
      if (awaiting === audioStep) {
        awaiting = null;
        controller.shown(performance.now());
      }
    };
    const onSeeking = () => {
      waiting = false;
      controller.restart(performance.now());
    };
    video.addEventListener('waiting', onWaiting);
    video.addEventListener('playing', onPlaying);
    video.addEventListener('seeking', onSeeking);

    let frameRequest: number | null = null;
    const frames = typeof video.requestVideoFrameCallback === 'function';
    const onFrame = () => {
      if (stopped) {
        return;
      }
      const now = performance.now();
      lastFrameAt = now;
      start(now);
      if (awaiting !== null && awaiting !== audioStep) {
        awaiting = null;
        controller.shown(now);
      }
      frameRequest = video.requestVideoFrameCallback(onFrame);
    };
    if (frames) {
      frameRequest = video.requestVideoFrameCallback(onFrame);
    }

    const probe = (bytes: number) => {
      const abort = new AbortController();
      probeAbort.current = abort;
      void measureProbe({
        bytes,
        timeoutMs: defaultNetworkParams.probeTimeoutMs,
        signal: abort.signal,
      }).then((outcome) => {
        if (probeAbort.current === abort) {
          probeAbort.current = null;
        }
        controller.probed(performance.now(), outcome);
      });
    };

    const timer = window.setInterval(() => {
      const now = performance.now();
      const state = read();
      if (state.current === null) {
        return;
      }
      if (state.current !== seenCurrent) {
        seenCurrent = state.current;
        awaiting = state.current;
        lastFrameAt = now;
        started = false;
      }
      const ahead = aheadOf(video);
      const active =
        !video.paused &&
        !video.seeking &&
        !video.ended &&
        (document.visibilityState === 'visible' ||
          state.pictureInPicture ||
          document.pictureInPictureElement === video);
      if (state.convert && state.mode === 'rate' && ahead >= selfLimitAhead) {
        meter.limit(now);
      }
      const { buffered } = video;
      const receiveRate =
        state.mode === 'rate' && !state.convert
          ? meter.throughput(now)
          : meter.delivered(now, defaultNetworkParams.rateWindowMs);
      const ladder = state.ladder.map((step, index) =>
        index === 0 && step.bitrate === null && state.topBitrate !== null
          ? { ...step, bitrate: state.topBitrate }
          : step,
      );
      const decision = controller.tick(
        now,
        {
          active,
          started,
          waiting: waiting && !excused(),
          lowReadyState: video.readyState < haveFutureData,
          // 音声だけの段と、コマごとのコールバックが無い端末では、コマの途絶を測らない。
          frameAgeMs: frames && state.current !== audioStep ? now - lastFrameAt : null,
          ahead,
          broadcastLag:
            state.mode === 'probe' && buffered.length > 0
              ? now / 1_000 - buffered.end(buffered.length - 1)
              : null,
          receiveRate,
          // 今の tenban と kyusu は、送り手の様子を画面に返さない (設計書 7.5)。
          server: null,
        },
        {
          ladder,
          current: state.current,
          automatic: state.automatic,
          mode: state.mode,
        },
      );
      // 試しの最中に足りない兆しが出たら、再生中の映像を細らせないよう打ち切る (余裕が無かったとみなす)。
      if (controller.probing() && controller.short()) {
        probeAbort.current?.abort();
      }
      if (
        !state.automatic &&
        controller.short() &&
        now - manualNoticeAt >= defaultNetworkParams.holdAfterSwitchMs
      ) {
        manualNoticeAt = now;
        shortManual();
      }
      if (decision.kind === 'step') {
        applyStep(decision.to);
        stepped({ to: decision.to, direction: decision.direction, reason: decision.reason });
      } else if (decision.kind === 'probe') {
        probe(decision.bytes);
      }
    }, defaultNetworkParams.tickMs);

    return () => {
      stopped = true;
      window.clearInterval(timer);
      if (frameRequest !== null) {
        video.cancelVideoFrameCallback(frameRequest);
      }
      video.removeEventListener('waiting', onWaiting);
      video.removeEventListener('playing', onPlaying);
      video.removeEventListener('seeking', onSeeking);
    };
  }, [video, controller, meter]);

  // 画面を閉じたら、取っている試しのデータを打ち切る。
  useEffect(
    () => () => {
      probeAbort.current?.abort();
    },
    [],
  );

  return {
    status: () => (video === null ? null : controller.status(performance.now())),
    onReading: (reading) => {
      const last = lastReading.current;
      if (reading.seeking) {
        lastReading.current = null;

        return;
      }
      if (last !== null && last.load === reading.load && reading.bytesRead > last.bytes) {
        meter.add(performance.now(), reading.bytesRead - last.bytes);
      }
      lastReading.current = { load: reading.load, bytes: reading.bytesRead };
    },
    onSegment: (segment) => meter.add(performance.now(), segment.bytes, segment.transferMs),
  };
}
