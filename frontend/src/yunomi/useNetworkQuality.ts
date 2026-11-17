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

  // started は、今の video で (段が変わったら、その段で) 再生が始まったか。始まるまでは判定しない。
  // 読み込みに時間がかかっても、始まるまでの詰まりを「足りない」と取り違えないよう、切り替え直後の無視時間は始まってから数える。
  // 段の切り替えは video を作り直さないこと (kyusu の段どうし) も、作り直すこと (変換と kyusu の間) もあるので、video の外で持つ。
  const started = useRef(false);

  // 経路の種類 (ライブと追っかけ) や段が変わったら、届いた量を数え直し、新しい段が始まるのを待つ。
  // 取っている試しのデータは打ち切る。前の段の受け取りと比べた結果を、新しい段に持ち込まないため。
  useEffect(() => {
    probeAbort.current?.abort();
    probeAbort.current = null;
    meter.reset();
    lastReading.current = null;
    started.current = false;
  }, [meter, options.mode, playback.current]);

  useEffect(() => {
    if (video === null) {
      return;
    }
    let waiting = false;
    let lastFrameAt = performance.now();
    started.current = false;
    // start は、最初のコマが映るか再生が始まったら呼ぶ。上げた段なら、映り出したことを知らせる。
    const start = (now: number) => {
      lastFrameAt = now;
      if (!started.current) {
        started.current = true;
        controller.restart(now);
        controller.shown(now);
      }
    };
    let stopped = false;
    let manualNoticeAt = -Infinity;

    // ライブでは、利用者はシークしない。シークは、プレイヤーが詰まりから戻ろうとして端へ移すものなので、詰まりとして数え続ける。
    // 数え直すと、細い回線で数秒おきに端へ移る間、切り替え直後の無視時間が続き、兆しを一度も見られない。
    const internalSeek = () => read().mode === 'probe';
    const onWaiting = () => {
      if (!video.paused && (!video.seeking || internalSeek()) && !excused()) {
        waiting = true;
      }
    };
    const onPlaying = () => {
      waiting = false;
      start(performance.now());
    };
    // 止めていた間や隠れていた間は、コマが来ないのが当たり前なので、動き出したら数え直す。
    const onResume = () => {
      lastFrameAt = performance.now();
    };
    const onSeeking = () => {
      if (internalSeek()) {
        return;
      }
      waiting = false;
      controller.restart(performance.now());
    };
    video.addEventListener('waiting', onWaiting);
    video.addEventListener('playing', onPlaying);
    // 選局やつなぎ直しで読み込み直したら、新しい映像が映り出してから数え直す (ライブのシークでは数え直さないため)。
    const onLoadStart = () => {
      waiting = false;
      started.current = false;
    };
    video.addEventListener('seeking', onSeeking);
    video.addEventListener('loadstart', onLoadStart);
    video.addEventListener('play', onResume);
    document.addEventListener('visibilitychange', onResume);

    let frameRequest: number | null = null;
    const frames = typeof video.requestVideoFrameCallback === 'function';
    const onFrame = () => {
      if (stopped) {
        return;
      }
      start(performance.now());
      frameRequest = video.requestVideoFrameCallback(onFrame);
    };
    if (frames) {
      frameRequest = video.requestVideoFrameCallback(onFrame);
    }

    const probe = (bytes: number) => {
      const abort = new AbortController();
      probeAbort.current = abort;
      const startedAt = performance.now();
      void measureProbe({
        bytes,
        timeoutMs: defaultNetworkParams.probeTimeoutMs,
        signal: abort.signal,
      }).then((outcome) => {
        if (probeAbort.current === abort) {
          probeAbort.current = null;
        }
        const now = performance.now();
        // 試しの間に映像が受け取った速さを添える。試しと映像は同じ回線を分け合う。
        controller.probed(
          now,
          outcome.kind === 'measured' && now > startedAt
            ? {
                ...outcome,
                alongsideBps:
                  (meter.receivedBetween(startedAt, now) * 8 * 1_000) / (now - startedAt),
              }
            : outcome,
        );
      });
    };

    const timer = window.setInterval(() => {
      const now = performance.now();
      const state = read();
      if (state.current === null) {
        return;
      }
      const ahead = aheadOf(video);
      const active =
        !video.paused &&
        (!video.seeking || state.mode === 'probe') &&
        !video.ended &&
        (document.visibilityState === 'visible' ||
          state.pictureInPicture ||
          document.pictureInPictureElement === video);
      if (!active) {
        lastFrameAt = now;
      }
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
          started: started.current,
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
      video.removeEventListener('loadstart', onLoadStart);
      video.removeEventListener('play', onResume);
      document.removeEventListener('visibilitychange', onResume);
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
