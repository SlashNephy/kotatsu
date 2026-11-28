// 映像だけの遅れを計測して飛び直し、デコードが追いつかない端末を見分ける (#1513、設計書 7.2)。
// video の要素にも React にも触れず、時刻と計測した値を渡されて決めるだけにする。要素を読んで渡すのは useVideoLag。

// VideoLagParams は、制御の値。秒の値は秒、時間の値はミリ秒で持つ。既定値は設計書の付録 A。
export type VideoLagParams = {
  // lagThreshold は、飛び直す遅れ。映像だけの遅れ (currentTime − 映ったフレームの mediaTime) がこれを超えたら飛び直す。
  lagThreshold: number;
  // jumpWidth は、飛び直しの幅。currentTime をこれだけ先へ進める。同じ値を代入し直しても飛んだことにならない。
  jumpWidth: number;
  // jumpHeadroom は、飛び先より先に要る、読み込んである量。読み込んだ範囲の外へ飛ぶと、変換のプレイヤーが配信を読み直す。
  jumpHeadroom: number;
  // minJumpIntervalMs は、飛び直しの最短間隔。
  minJumpIntervalMs: number;
  // jumpWindowMs と heavyJumps は、飛び直しを数えるウィンドウと、重いとみなす飛び直しの回数。
  jumpWindowMs: number;
  heavyJumps: number;
  // dropWindowMs と dropMinFrames は、破棄したフレームを数えるウィンドウと、ウィンドウの最低フレーム数。
  dropWindowMs: number;
  dropMinFrames: number;
  // heavyDropRatio は、重いとみなす破棄した割合。
  heavyDropRatio: number;
  // noSpeedUpMs は、遅れが飛び直す遅れを超えたか飛び直したあと、追いつくための速め (#1514) をしない時間。
  noSpeedUpMs: number;
  // selfStallGraceMs は、自分で飛び直したあと、詰まりを回線の詰まりとして数えない時間。
  selfStallGraceMs: number;
};

// defaultVideoLagParams は、既定値。どれも初期値で、実機で計測して決め直す (設計書の付録 A)。
export const defaultVideoLagParams: VideoLagParams = {
  // 開発用の環境 (ヘッドレスの Chromium、変換の経路) では、無事なときの遅れの最大は約 0.03 秒、GPU のプロセスを絞っても約 0.1 秒だった。
  lagThreshold: 0.15,
  // 飛ぶたびにこの分の音が欠けるので、1 フレーム (約 0.033 秒) より大きく、聞いて分からない程度に小さくする。
  jumpWidth: 0.1,
  jumpHeadroom: 0.1,
  // 変換の経路のキーフレームは約 0.5 秒おき。飛んだ後にデコーダが出し直して落ち着くまでより十分長くする。
  minJumpIntervalMs: 5_000,
  // 最短間隔の 12 倍。1 回の重さ (GC、タブの切り替え) で段階を下げないよう、3 回で重いとみなす。
  jumpWindowMs: 60_000,
  heavyJumps: 3,
  // 29.97 fps で 10 秒なら約 300 フレーム。半分以上を見て割合を出す。
  dropWindowMs: 10_000,
  dropMinFrames: 150,
  heavyDropRatio: 0.3,
  noSpeedUpMs: 10_000,
  selfStallGraceMs: 3_000,
};

// FrameInput は、映ったフレーム 1 つ分の計測した値。
export type FrameInput = {
  // lag は、映像だけの遅れ (秒)。
  lag: number;
  // ahead は、今の位置から先に読み込んである量 (秒)。
  ahead: number;
};

// JumpCause は、飛び直した理由。lag は計測した遅れ、stall は遅れを測れない端末での詰まり。
export type JumpCause = 'lag' | 'stall';

// VideoLagStatus は、再生の情報に出す様子。
export type VideoLagStatus = {
  // measurable は、この端末で遅れを測れるか。
  measurable: boolean;
  // lag は、最後に計測した遅れ (秒)。まだ計測していなければ null。
  lag: number | null;
  // maxLag は、今の video で計測した遅れの最大 (秒)。
  maxLag: number;
  // jumps は、今の video で飛び直した回数。
  jumps: number;
  // dropRatio は、直近のウィンドウで破棄したフレームの割合。数えられなければ null。
  dropRatio: number | null;
};

// VideoLagController は、遅れの記録と飛び直しの決まりを持つ。時刻 now は、どれもミリ秒。
export type VideoLagController = {
  // frame は、映ったフレームの遅れを通知し、飛び直すかを返す。飛んだら、呼んだ側が jumped を呼ぶ。
  frame(now: number, input: FrameInput): boolean;
  // seeking は、seek が始まったことを通知する。次の 1 フレームは、飛ぶ前の時刻を持っていることがあるので計測しない。
  seeking(): void;
  // stall は、再生中に止まったことを通知する。遅れを測れない端末では、動き出したときに 1 回飛び直す。
  stall(): void;
  // resumed は、止まった後に動き出したことを通知し、飛び直すかを返す。
  resumed(now: number, ahead: number): boolean;
  // jumped は、飛び直したことを通知する。counted は、重さの判定に数えるか (倍速や早送りでなく、計測した遅れで飛んだとき)。
  jumped(now: number, cause: JumpCause, counted: boolean): void;
  // quality は、破棄したフレームと出したフレームの累計 (getVideoPlaybackQuality) を通知する。数えてよい間 (倍速や早送りでなく再生中) だけ呼ぶ。
  quality(now: number, total: number, dropped: number): void;
  // forgetQuality は、破棄したフレームの記録を破棄する。数えない間があったら呼び、その前後の差を数えないようにする。
  forgetQuality(): void;
  // heavy は、その端末にはその段階が重いとみなせるかを返す。
  heavy(now: number): boolean;
  // reset は、重さの判定に数えたものを破棄する。重いと判定したら呼ぶ。再生の情報に出す遅れの最大と飛び直した回数は残す。
  // 経路を替えたり、MediaSource を作り直したりしたら (video が変わったら)、制御ごと作り直す。
  reset(): void;
  // speedUpAllowed は、追いつくための速めをしてよいかを返す。
  speedUpAllowed(now: number): boolean;
  // excusesStall は、今の詰まりが、自分で飛び直したことで起きたものとみなせるかを返す。
  excusesStall(now: number): boolean;
  status(now: number): VideoLagStatus;
};

type QualitySample = { at: number; total: number; dropped: number };

export type VideoLagOptions = {
  // measurable は、この端末で遅れを測れるか (lagMeasurable)。
  measurable: boolean;
  params?: VideoLagParams;
};

// createVideoLagController は、遅れの制御を作る。
export function createVideoLagController(options: VideoLagOptions): VideoLagController {
  const params = options.params ?? defaultVideoLagParams;
  const { measurable } = options;
  let lastJumpAt: number | null = null;
  let skipNext = true;
  let stalled = false;
  let holdSpeedUntil = -Infinity;
  let excuseUntil = -Infinity;
  let countedJumps: number[] = [];
  let samples: QualitySample[] = [];
  let lastLag: number | null = null;
  let maxLag = 0;
  let jumps = 0;

  const canJump = (now: number, ahead: number) =>
    (lastJumpAt === null || now - lastJumpAt >= params.minJumpIntervalMs) &&
    ahead >= params.jumpWidth + params.jumpHeadroom;

  const prune = (now: number) => {
    countedJumps = countedJumps.filter((at) => now - at < params.jumpWindowMs);
    // ウィンドウの始まりの値と比較するため、ウィンドウより古いものを 1 つだけ残す。
    while (samples.length > 1 && now - samples[1]!.at >= params.dropWindowMs) {
      samples.shift();
    }
  };

  // dropRatio は、直近のウィンドウで破棄したフレームの割合。ウィンドウの最低フレーム数に満たなければ null。
  const dropRatio = (now: number): number | null => {
    prune(now);
    const first = samples[0];
    const last = samples.at(-1);
    if (first === undefined || last === undefined) {
      return null;
    }
    const frames = last.total - first.total;
    if (frames < params.dropMinFrames) {
      return null;
    }

    return (last.dropped - first.dropped) / frames;
  };

  return {
    frame(now, input) {
      if (skipNext) {
        skipNext = false;

        return false;
      }
      // 遅れを測れない端末では、mediaTime に遅れが出ないので、計測した値で飛ばない。
      if (!measurable) {
        return false;
      }
      lastLag = input.lag;
      maxLag = Math.max(maxLag, input.lag);
      if (input.lag <= params.lagThreshold) {
        return false;
      }
      holdSpeedUntil = Math.max(holdSpeedUntil, now + params.noSpeedUpMs);

      return canJump(now, input.ahead);
    },
    seeking() {
      skipNext = true;
    },
    stall() {
      if (!measurable) {
        stalled = true;
      }
    },
    resumed(now, ahead) {
      if (!stalled) {
        return false;
      }
      stalled = false;

      return canJump(now, ahead);
    },
    jumped(now, cause, counted) {
      lastJumpAt = now;
      jumps += 1;
      skipNext = true;
      excuseUntil = now + params.selfStallGraceMs;
      holdSpeedUntil = Math.max(holdSpeedUntil, now + params.noSpeedUpMs);
      // 測れない端末の、詰まりのたびの飛び直しは数えない。
      if (cause === 'lag' && counted) {
        countedJumps.push(now);
      }
    },
    quality(now, total, dropped) {
      const last = samples.at(-1);
      // 累計が戻ったら (作り直した video)、前の記録と比較しない。ウィンドウより長く間が空いたときも (スリープ、タイマーの間引き)、
      // 空いた間をならして割合を薄めないよう、前の記録と比較しない。
      if (
        last !== undefined &&
        (total < last.total || dropped < last.dropped || now - last.at >= params.dropWindowMs)
      ) {
        samples = [];
      }
      samples.push({ at: now, total, dropped });
      prune(now);
    },
    forgetQuality() {
      samples = [];
    },
    heavy(now) {
      prune(now);
      if (countedJumps.length >= params.heavyJumps) {
        return true;
      }
      const ratio = dropRatio(now);

      return ratio !== null && ratio > params.heavyDropRatio;
    },
    reset() {
      countedJumps = [];
      samples = [];
      stalled = false;
      skipNext = true;
    },
    speedUpAllowed(now) {
      return now >= holdSpeedUntil;
    },
    excusesStall(now) {
      return now < excuseUntil;
    },
    status(now) {
      return { measurable, lag: lastLag, maxLag, jumps, dropRatio: dropRatio(now) };
    },
  };
}

// lagOf は、映像だけの遅れ (秒) を返す。負なら 0。
export function lagOf(currentTime: number, mediaTime: number): number {
  return Math.max(currentTime - mediaTime, 0);
}

// NavigatorLike は、端末を見分けるのに読む navigator の一部。
export type NavigatorLike = {
  userAgent: string;
  userAgentData?: { platform: string } | undefined;
};

// lagMeasurable は、この端末で映像だけの遅れを測れるかを返す。Windows のブラウザは映像を別の面に出すので、
// 面の側が遅れても mediaTime に出ない。userAgentData があればその platform で、無ければ User-Agent で見分ける。
export function lagMeasurable(navigator: NavigatorLike): boolean {
  const platform = navigator.userAgentData?.platform;
  if (platform !== undefined && platform !== '') {
    return platform !== 'Windows';
  }

  return !/Windows/.test(navigator.userAgent);
}
