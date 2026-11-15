// ライブで貯める量 (目標量) を、止まり方と届き方から決め、再生の速さで寄せる制御 (#1514、設計書 7.2)。
// video の要素にも React にも触れず、時刻と測った値を渡されて決めるだけにする。要素を読んで渡すのは useLiveCatchUp。
// 映像だけの遅れ (#1513) と、MPEG-2 を WASM でデコードする経路 (#1516) も、この制御を使う。

// LiveBufferParams は、制御の値。秒の値は秒、時間の値はミリ秒で持つ。既定値は設計書の付録 A。
export type LiveBufferParams = {
  // minTarget は、初期の最小量。目標量の初期値で、縮めるときの最小。
  minTarget: number;
  // maxTarget は、目標量と下限の上限。
  maxTarget: number;
  // stallIncrease は、説明がつかない詰まりで目標量に足す量。
  stallIncrease: number;
  // shrinkRatio と minShrink は、無事が続いたときに、今の目標量から縮める割合と、最低の縮み幅。
  shrinkRatio: number;
  minShrink: number;
  // shrinkAfterMs は、最後に止まってから、目標量を縮め始めるまでの無事。
  shrinkAfterMs: number;
  // floorStep は、止まったときに、止まった高さの上に置く次の下限の幅 (下限の持ち上げ幅)。下げ直すときも、この幅で下げる。
  floorStep: number;
  // floorDecayAfterMs は、下限を一段下げてよいとみなすまでの無事。
  floorDecayAfterMs: number;
  // jumpMargin は、バッファが「目標量 + この余り」を超えたら、末尾 − 目標量へ飛ぶ余り。
  jumpMargin: number;
  // speedUpRate と slowDownRate は、溜まりすぎのときと、足りないときの再生の速さ。
  speedUpRate: number;
  slowDownRate: number;
  // gapMemoryMs は、届く間隔の最大を覚えておく長さ。
  gapMemoryMs: number;
  // maxCountedGap は、揺れとして覆う届く間隔の最大 (秒)。これより長いのは途絶えなので、覆わない。
  maxCountedGap: number;
  // coverFactor と coverAdd は、覆う量 = 間隔 × 係数 + 足し分 の係数と足し分。
  coverFactor: number;
  coverAdd: number;
  // lowWindowMs は、バッファの最小値と届く間隔の最大を見る直近の長さ (バッファの低い所を見る長さ)。
  lowWindowMs: number;
  // retargetIntervalMs は、目標量を決め直す間隔。止まったときは、待たずに決め直す。
  retargetIntervalMs: number;
  // selfInflictedGraceMs は、選局、飛び、MediaSource の作り直しの直後に、詰まりを数えない時間。
  selfInflictedGraceMs: number;
  // speedUpStartRatio と speedUpStopRatio は、比べる値が「目標量 × 倍率」を超えたら速め、以下になったらやめる倍率。
  speedUpStartRatio: number;
  speedUpStopRatio: number;
  // webkitStartMargin と webkitStopRatio は、Safari と iPad/iPhone で、速め始める「目標量 + 余り」と、やめる倍率。
  webkitStartMargin: number;
  webkitStopRatio: number;
  // tickMs は、塊が届かない間も、速さを判断する周期 (判断の刻み)。
  tickMs: number;
  // chaseEndRatio は、追っかけで、バッファが「目標量 × この倍率」以下に戻ったらライブに戻す倍率。
  chaseEndRatio: number;
};

// defaultLiveBufferParams は、制御の既定値。どれも初期値で、実機で測って決め直す (設計書の付録 A)。
// 変換の経路の断片は約 0.5 秒 (MPEG-2 の GOP ごと) で、ほぼ 0.5 秒おきに届くことを、開発用の環境で確かめた。
export const defaultLiveBufferParams: LiveBufferParams = {
  // 断片の 2 つ分。止まれば伸びるので、小さい側から始める。
  minTarget: 1,
  // 変換のプレイヤーは、再生位置より 8 秒先まで貯めると変換を止める (maxAheadSeconds)。飛ぶ所 (上限 + 飛ぶ余り) をそれより手前に置く。
  maxTarget: 5,
  stallIncrease: 1,
  shrinkRatio: 0.1,
  minShrink: 0.05,
  shrinkAfterMs: 30_000,
  floorStep: 0.5,
  floorDecayAfterMs: 10 * 60_000,
  jumpMargin: 2.5,
  // 今までの追いつきの速さ (#495) を起点にする。
  speedUpRate: 1.1,
  slowDownRate: 0.95,
  gapMemoryMs: 60_000,
  // 覆う量 (3 × 1.25 + 0.25 = 4 秒) が上限より小さくなるようにする。
  maxCountedGap: 3,
  coverFactor: 1.25,
  coverAdd: 0.25,
  lowWindowMs: 4_000,
  retargetIntervalMs: 5_000,
  // 開発用の環境では、読み込んでから最初のコマが映るまで 1.2〜2.7 秒だった。
  selfInflictedGraceMs: 5_000,
  speedUpStartRatio: 1.25,
  speedUpStopRatio: 1.1,
  webkitStartMargin: 1,
  webkitStopRatio: 1.05,
  tickMs: 200,
  chaseEndRatio: 1.5,
};

// RateMode は、再生の速さの寄せ方。
export type RateMode = 'normal' | 'up' | 'down';

// RateInput は、速さを決めるときに渡す今の様子。
export type RateInput = {
  // buffer は、今のバッファ (buffered の末尾 − currentTime、秒)。
  buffer: number;
  // conservative は、遅くせず、速さを変える回数を減らす寄せ方にするか。Safari と iPad/iPhone、タブが隠れている間。
  conservative: boolean;
  // speedUpAllowed は、速めてよいか。映像だけ遅れている間 (#1513) は速めない。
  speedUpAllowed: boolean;
};

// RateDecision は、決めた再生の速さと、飛ぶかどうか。
export type RateDecision = {
  rate: number;
  // jump は、末尾 − 目標量 へ飛ぶべきかどうか。飛んだら、呼んだ側が selfInflicted を呼ぶ。
  jump: boolean;
};

// LiveBufferStatus は、再生の情報に出す、制御の様子。
export type LiveBufferStatus = {
  target: number;
  floor: number;
  // maxGap は、覚えている届く間隔の最大 (秒)。いま空いている最中の間隔も含める。
  maxGap: number;
  // rateChanges は、速さを変えた回数。
  rateChanges: number;
  // stalls は、数えた詰まりの回数。
  stalls: number;
  // chasing は、一時停止の後で、わざと遅れて見ている (追っかけ) か。
  chasing: boolean;
  // learned は、初期の最小量より上の下限を覚えているか。覚えた下限を捨てるボタンは、このときだけ出す。
  learned: boolean;
};

type Gap = { at: number; gap: number };
type Sample = { at: number; buffer: number };

// LiveBufferController は、目標量と下限を持ち、事象を受けて決め直す。時刻 now は、どれもミリ秒。
export type LiveBufferController = {
  // arrive は、塊が届いたことを知らせる。bufferBefore は、届く直前のバッファ。
  arrive(now: number, bufferBefore: number): void;
  // sample は、映ったコマや判断の刻みで測ったバッファを知らせる。
  sample(now: number, buffer: number): void;
  // forgetArrival は、前の到着時刻を忘れる。シーク、一時停止、タブが隠れたとき。
  forgetArrival(): void;
  // pause は、一時停止したことを知らせる。再開したら、追いつくまで追っかけとして扱い、飛ばない。
  pause(): void;
  // selfInflicted は、選局、飛び、MediaSource の作り直しを知らせる。前の到着時刻を忘れ、しばらく詰まりを数えない。
  selfInflicted(now: number): void;
  // stall は、止まったことを知らせ、数えたら目標量を決め直す。数えたかを返す。
  stall(now: number): boolean;
  // tick は、判断の刻みごとに呼ぶ。決め直す間隔がたっていれば、目標量を決め直す。
  tick(now: number): void;
  // decide は、次の再生の速さと、飛ぶかどうかを決める。
  decide(now: number, input: RateInput): RateDecision;
  // forgetFloor は、覚えた下限を捨て、目標量を初期の最小量まで詰める。放送の今へ戻すのは、呼んだ側。
  forgetFloor(now: number): void;
  // coverage は、今の覆う量。
  coverage(now: number): number;
  status(now: number): LiveBufferStatus;
  readonly target: number;
  readonly floor: number;
};

export type LiveBufferOptions = {
  now: number;
  // storedFloor は、端末に覚えた下限。なければ null。
  storedFloor: number | null;
  params?: LiveBufferParams;
  // onFloorChange は、下限が変わったときに呼ぶ。初期の最小量まで戻ったら null を渡す。端末に覚えるのは呼んだ側。
  onFloorChange?: (floor: number | null) => void;
};

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

// clampFloor は、端末に覚えた下限を、初期の最小量から上限の間に丸める。読めない値は null。
export function clampFloor(value: number | null, params = defaultLiveBufferParams): number | null {
  if (value === null || !Number.isFinite(value)) {
    return null;
  }

  return clamp(value, params.minTarget, params.maxTarget);
}

// createLiveBufferController は、目標量の制御を作る。目標量は、覚えた下限から始める。
// 毎回初期値から始めると、開いてすぐに決まって 1 回止まるため。
export function createLiveBufferController(options: LiveBufferOptions): LiveBufferController {
  const params = options.params ?? defaultLiveBufferParams;
  const initialFloor = clampFloor(options.storedFloor, params) ?? params.minTarget;
  let floor = initialFloor;
  let target = initialFloor;
  let lastStallAt = options.now;
  let floorChangedAt = options.now;
  let lastRetargetAt = options.now;
  let lastArrival: number | null = null;
  let graceUntil = options.now + params.selfInflictedGraceMs;
  let gaps: Gap[] = [];
  let samples: Sample[] = [];
  let mode: RateMode = 'normal';
  let rateChanges = 0;
  let stalls = 0;
  let chasing = false;

  const setFloor = (now: number, next: number) => {
    if (next === floor) {
      return;
    }
    floor = next;
    floorChangedAt = now;
    options.onFloorChange?.(floor > params.minTarget ? floor : null);
  };

  const prune = (now: number) => {
    const horizon = now - Math.max(params.gapMemoryMs, params.lowWindowMs);
    if (gaps.length > 0 && gaps[0]!.at < horizon) {
      gaps = gaps.filter((gap) => gap.at >= horizon);
    }
    const lowHorizon = now - params.lowWindowMs;
    if (samples.length > 0 && samples[0]!.at < lowHorizon) {
      samples = samples.filter((sample) => sample.at >= lowHorizon);
    }
  };

  // recordedGap は、直近 windowMs の間に記録した届く間隔の最大。
  const recordedGap = (now: number, windowMs: number) => {
    let max = 0;
    for (const gap of gaps) {
      if (gap.at >= now - windowMs && gap.gap > max) {
        max = gap.gap;
      }
    }

    return max;
  };

  // openGap は、いま空いている最中の間隔 (秒)。前の到着を覚えていなければ 0。
  const openGap = (now: number) => (lastArrival === null ? 0 : (now - lastArrival) / 1_000);

  // coveredGap は、覆う量に通す間隔。直近の覚える長さの最大と、いま空いている最中の間隔 (上限まで) の長い方。
  const coveredGap = (now: number) =>
    Math.max(recordedGap(now, params.gapMemoryMs), Math.min(openGap(now), params.maxCountedGap));

  const coverage = (now: number) =>
    clamp(
      coveredGap(now) * params.coverFactor + params.coverAdd,
      params.minTarget,
      params.maxTarget,
    );

  const retarget = (now: number, stalled: boolean) => {
    lastRetargetAt = now;
    const cover = coverage(now);
    if (stalled) {
      lastStallAt = now;
      if (cover > target) {
        // 空きで説明がつく詰まり。下限は動かさず、空きが来なくなれば、覚える長さがたって忘れ、縮む。
        target = clamp(Math.max(cover, floor), params.minTarget, params.maxTarget);
      } else {
        // 説明がつかない詰まり。止まった高さのすぐ上を次の下限にする。
        setFloor(now, Math.min(Math.max(floor, target + params.floorStep), params.maxTarget));
        target = Math.min(target + params.stallIncrease, params.maxTarget);
      }

      return;
    }
    const quiet = now - lastStallAt;
    if (quiet >= params.floorDecayAfterMs && now - floorChangedAt >= params.floorDecayAfterMs) {
      const lowered = Math.max(floor - params.floorStep, cover, params.minTarget);
      if (lowered < floor) {
        setFloor(now, lowered);
      }
    }
    const shrunk =
      quiet < params.shrinkAfterMs
        ? target
        : target - Math.max(target * params.shrinkRatio, params.minShrink);
    target = clamp(Math.max(shrunk, floor, cover), params.minTarget, params.maxTarget);
  };

  return {
    arrive(now, bufferBefore) {
      if (lastArrival !== null) {
        const gap = (now - lastArrival) / 1_000;
        // 上限より長いのは揺れではなく途絶えなので、覆わない。
        if (gap <= params.maxCountedGap) {
          gaps.push({ at: now, gap });
        }
      }
      lastArrival = now;
      samples.push({ at: now, buffer: bufferBefore });
      prune(now);
    },
    sample(now, buffer) {
      samples.push({ at: now, buffer });
      prune(now);
    },
    forgetArrival() {
      lastArrival = null;
    },
    pause() {
      lastArrival = null;
      chasing = true;
    },
    selfInflicted(now) {
      lastArrival = null;
      chasing = false;
      samples = [];
      graceUntil = now + params.selfInflictedGraceMs;
    },
    stall(now) {
      if (now < graceUntil) {
        return false;
      }
      stalls += 1;
      retarget(now, true);

      return true;
    },
    tick(now) {
      prune(now);
      if (now - lastRetargetAt >= params.retargetIntervalMs) {
        retarget(now, false);
      }
    },
    decide(now, input) {
      prune(now);
      // 追っかけでは飛ばず、遅くもしない。追い越して止まる前に、ライブに戻す。
      if (chasing && input.buffer <= target * params.chaseEndRatio) {
        chasing = false;
      }
      if (!chasing && input.buffer > target + params.jumpMargin) {
        mode = 'normal';

        return { rate: 1, jump: true };
      }
      // 届いた直後のバッファは束のたびに跳ねるので、直近の低い所に、同じ長さの届く間隔の最大を足して比べる。
      let low = input.buffer;
      for (const sample of samples) {
        if (sample.at >= now - params.lowWindowMs && sample.buffer < low) {
          low = sample.buffer;
        }
      }
      // いま空いている最中の間隔は、覚える長さの間の最大までしか足さない。それより長く空いたら、足さずに遅くして持たせる。
      const gap = Math.max(
        recordedGap(now, params.lowWindowMs),
        Math.min(openGap(now), recordedGap(now, params.gapMemoryMs)),
      );
      const compared = low + gap;
      const next = nextMode(
        mode,
        compared,
        target,
        { ...input, conservative: input.conservative || chasing },
        params,
      );
      if (next !== mode) {
        rateChanges += 1;
        mode = next;
      }

      return { rate: rateOf(mode, params), jump: false };
    },
    forgetFloor(now) {
      setFloor(now, params.minTarget);
      target = params.minTarget;
      lastRetargetAt = now;
    },
    coverage,
    status(now) {
      return {
        target,
        floor,
        maxGap: Math.max(recordedGap(now, params.gapMemoryMs), openGap(now)),
        rateChanges,
        stalls,
        chasing,
        learned: floor > params.minTarget,
      };
    },
    get target() {
      return target;
    },
    get floor() {
      return floor;
    },
  };
}

// nextMode は、比べる値 compared と目標量 target から、次の寄せ方を決める。
// 速めるときは、やめる所を始める所より低く置き、境目での往復を防ぐ。遅くしているときは、目標量に戻ったらすぐ等速に戻す。
export function nextMode(
  current: RateMode,
  compared: number,
  target: number,
  input: Pick<RateInput, 'conservative' | 'speedUpAllowed'>,
  params: LiveBufferParams = defaultLiveBufferParams,
): RateMode {
  const [start, stop] = input.conservative
    ? [target + params.webkitStartMargin, target * params.webkitStopRatio]
    : [target * params.speedUpStartRatio, target * params.speedUpStopRatio];
  if (!input.conservative && compared < target) {
    return 'down';
  }
  if (!input.speedUpAllowed) {
    return 'normal';
  }
  if (current === 'up' ? compared > stop : compared > start) {
    return 'up';
  }

  return 'normal';
}

// rateOf は、寄せ方の再生の速さ。
export function rateOf(mode: RateMode, params: LiveBufferParams = defaultLiveBufferParams): number {
  switch (mode) {
    case 'up':
      return params.speedUpRate;
    case 'down':
      return params.slowDownRate;
    default:
      return 1;
  }
}

// prefersConservativeRate は、遅くせず、速さを変える回数を減らすブラウザか。Safari と、iPad/iPhone のすべてのブラウザ (中身が WebKit)。
// WebKit は、再生中に playbackRate を変えるたびに、音と映像がつまずくことがある (要確認)。
// iPadOS の Safari は Mac の User-Agent を名乗るので、タッチの点の数で見分ける。
export function prefersConservativeRate(userAgent: string, maxTouchPoints: number): boolean {
  if (/iPhone|iPad|iPod/.test(userAgent)) {
    return true;
  }
  if (/Macintosh/.test(userAgent) && maxTouchPoints > 1) {
    return true;
  }

  return (
    /AppleWebKit/.test(userAgent) &&
    /Safari/.test(userAgent) &&
    !/Chrome|Chromium|CriOS|FxiOS|Edg|OPR|Firefox|Android/.test(userAgent)
  );
}

const floorStorageKey = 'kotatsu.player.liveBufferFloor';

// loadLiveBufferFloor は、端末に覚えた下限を読み、初期の最小量から上限の間に丸める。読めない、覚えていなければ null。
export function loadLiveBufferFloor(
  storage: Pick<Storage, 'getItem'> | null,
  params = defaultLiveBufferParams,
): number | null {
  try {
    const raw = storage?.getItem(floorStorageKey);
    if (raw == null) {
      return null;
    }

    return clampFloor(Number(raw), params);
  } catch {
    return null;
  }
}

// saveLiveBufferFloor は、下限を端末に覚える。null なら消す。書けなければ、次に開いたときは初期の最小量から始めるだけ。
export function saveLiveBufferFloor(
  storage: Pick<Storage, 'setItem' | 'removeItem'> | null,
  floor: number | null,
): void {
  try {
    if (floor === null) {
      storage?.removeItem(floorStorageKey);
    } else {
      storage?.setItem(floorStorageKey, String(floor));
    }
  } catch {
    // 書けなくても、再生は続ける。
  }
}
