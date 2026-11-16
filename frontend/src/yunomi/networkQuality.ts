// 回線の側の兆しで画質の「自動」の段を下げ、試しのデータや受け取りの速さで余裕を確かめてから上げる (#1513、設計書 7.1、7.5)。
// video の要素にも React にも触れず、時刻と測った値を渡されて決めるだけにする。要素を読んで渡すのは useNetworkQuality。

// NetworkParams は、制御の値。秒の値は秒、時間の値はミリ秒、速さは bps で持つ。既定値は設計書の付録 A。
export type NetworkParams = {
  // tickMs は、回線の側の兆しを見る間隔 (回線の判定の刻み)。
  tickMs: number;
  // ignoreAfterSwitchMs は、選局、シーク、段の切り替えの直後に数えない時間 (切り替え直後の無視時間)。
  ignoreAfterSwitchMs: number;
  // stallMs は、readyState が HAVE_FUTURE_DATA 未満の状態を、詰まりとみなすまでの時間 (詰まりとみなす時間)。
  stallMs: number;
  // frameGapMs は、次のコマが来ないのを、映像だけ止まったとみなすまでの時間 (コマの途絶とみなす時間)。
  frameGapMs: number;
  // drainBelow、drainDurationMs、drainRate は、バッファの減少を「足りない」とみなす、見るバッファの量 (秒)、減少の継続時間、
  // 減少の速さの閾値 (1 秒あたりの秒)。
  drainBelow: number;
  drainDurationMs: number;
  drainRate: number;
  // lagWindowMs、lagEdgeMs、lagGrowth、lagResetGrowth は、放送に対する遅れの伸びを見る窓、窓の端の区間、伸びの閾値 (秒)、
  // 端が戻ったとみなす伸び (秒)。
  lagWindowMs: number;
  lagEdgeMs: number;
  lagGrowth: number;
  lagResetGrowth: number;
  // backlogDurationMs は、サーバのバックログが伸び続けたら足りないとみなす時間 (バックログの継続時間)。
  backlogDurationMs: number;
  // margin は、受け取りの速さのうち、段に使ってよい割合 (余白の係数)。
  margin: number;
  // rateWindowMs と rateShortWindows は、受け取りの速さの窓と、その不足が続いたら下げるまでの窓の数 (速さ不足の継続時間)。
  rateWindowMs: number;
  rateShortWindows: number;
  // encodeSlowSpeed と encodeSlowDurationMs は、サーバのエンコードが追いつかないとみなす、エンコードの速さの閾値と継続時間。
  encodeSlowSpeed: number;
  encodeSlowDurationMs: number;
  // unknownRateSteps は、受け取りの速さが分からないときに一度に下げる段の数 (速さが分からないときに下げる段数)。
  unknownRateSteps: number;
  // audioConsiderMs、audioWindowMs、audioRatio は、最も低い段に移ってから音声だけを考えるまでの時間、音声だけを判定する窓、
  // その窓のうち「足りない」だった時間の割合 (音声だけにする割合)。
  audioConsiderMs: number;
  audioWindowMs: number;
  audioRatio: number;
  // holdAfterSwitchMs と holdAfterNoSlotMs は、切り替え後の据え置きと、枠不足のあとの据え置き。
  holdAfterSwitchMs: number;
  holdAfterNoSlotMs: number;
  // upHeadroom と upDurationMs は、受け取りの速さで上げる経路の、上げる余裕の倍率と、上げる余裕の継続時間。
  upHeadroom: number;
  upDurationMs: number;
  // probeAfterOkMs は、試しのデータで上げる経路で、足りない兆しが無い状態がこれだけ続いたら試す時間 (試すまでの無事の時間)。
  probeAfterOkMs: number;
  // probeHeadroom、probeSeconds、probeMinBytes、probeMaxBytes、probeTimeoutMs は、試しの余裕の倍率、試しの量の時間、
  // 試しの量の下限と上限、試しの打ち切り。
  probeHeadroom: number;
  probeSeconds: number;
  probeMinBytes: number;
  probeMaxBytes: number;
  probeTimeoutMs: number;
  // probeValidityMs は、試しで測った速さを使う期間 (測った値の有効期間)。
  probeValidityMs: number;
  // postUpWatchMs、backoffMultiplier、backoffMaxMs は、上げ直後の見張り時間、待ちの倍率、待ちの上限。
  postUpWatchMs: number;
  backoffMultiplier: number;
  backoffMaxMs: number;
  // upToDisplayMs は、段を上げるよう求めてから、新しい段が映り出すのを待つ時間 (上げてから映るまでの上限)。
  upToDisplayMs: number;
  // defaultTopBitrate は、最高画質 (オリジナル) のビットレートが分からず、足りていた間の受け取りの速さも測っていないときに使う値。
  defaultTopBitrate: number;
};

// defaultNetworkParams は、既定値。どれも初期値で、実機で測って決め直す (設計書の付録 A)。
export const defaultNetworkParams: NetworkParams = {
  tickMs: 500,
  // 低遅延モードの目標量 (初期 1 秒、上限 5 秒) を貯め、kyusu の経路で最初のセグメントが映るまでより長くする。
  ignoreAfterSwitchMs: 8_000,
  // 1 コマ (約 0.033 秒) と、変換の経路の断片の間隔 (約 0.5 秒) より十分長くする。
  stallMs: 1_000,
  frameGapMs: 1_500,
  drainBelow: 4,
  drainDurationMs: 4_000,
  // 低遅延モードで速めているときの減り方 (1.1 − 1 = 0.1) より大きくする。
  drainRate: 0.2,
  // kyusu のセグメント (2 秒) の数倍にし、端の区間をセグメントより長く、窓の半分より短くする。
  lagWindowMs: 20_000,
  lagEdgeMs: 5_000,
  lagGrowth: 1.5,
  lagResetGrowth: 5,
  backlogDurationMs: 3_000,
  margin: 0.8,
  // 窓 × 数 (6 秒) を、録画の再生で先に溜める量 (変換の経路の maxAheadSeconds、8 秒) より短くする。
  rateWindowMs: 3_000,
  rateShortWindows: 2,
  encodeSlowSpeed: 0.9,
  encodeSlowDurationMs: 5_000,
  // オリジナル (13〜24 Mbps) と kyusu の 1080p (約 6.1 Mbps) の差は、ほかの段どうしの差 (約 1.7〜2 倍) より大きいので、
  // オリジナルからも 1 段で足りるとみなす。
  unknownRateSteps: 1,
  audioConsiderMs: 30_000,
  audioWindowMs: 60_000,
  audioRatio: 0.5,
  holdAfterSwitchMs: 15_000,
  holdAfterNoSlotMs: 60_000,
  upHeadroom: 1.5,
  upDurationMs: 10_000,
  probeAfterOkMs: 30_000,
  probeHeadroom: 1.3,
  probeSeconds: 2,
  probeMinBytes: 256 * 1024,
  // tenban が返す上限 (8 MiB) の半分。
  probeMaxBytes: 4 * 1024 * 1024,
  probeTimeoutMs: 4_000,
  probeValidityMs: 20_000,
  postUpWatchMs: 30_000,
  backoffMultiplier: 2,
  backoffMaxMs: 5 * 60_000,
  upToDisplayMs: 15_000,
  // 地上波と BS の TS (13〜24 Mbps、設計書 7.1) の間。
  defaultTopBitrate: 20_000_000,
};

// audioStep は、段の表の、音声だけの段の名前。kyusu の映像を送らない画質の名前と同じ。
export const audioStep = 'audio';
// audioOnlyBitrate は、音声だけの段のビットレート (bps)。kyusu の音声の 128 kbps。
export const audioOnlyBitrate = 128_000;

// Step は、段の表の 1 段。高い順に並べて渡す。bitrate は映像と音声を足した速さ (bps)。分からなければ (オリジナル) null。
export type Step = { name: string; bitrate: number | null };

// NetworkMode は、上げ方。probe は試しのデータで (ライブ。実時間でしか届かず、受け取りの速さでは余裕が分からない)、
// rate は受け取りの速さで (録画と追っかけ。先に読むので、受け取りの速さで余裕が分かる) 確かめる。
export type NetworkMode = 'probe' | 'rate';

// ServerReport は、送り手が知らせる様子。今の tenban と kyusu は知らせないので、使っていない (設計書 7.5)。
export type ServerReport = {
  // backlogBytes は、送れずに溜まっている量。分からなければ null。
  backlogBytes: number | null;
  // dropped は、サーバが塊を捨てたか。
  dropped: boolean;
  // encodeSpeed は、エンコードの速さ (出した映像の時刻の進み ÷ 実時間)。0 は測れていない。分からなければ null。
  encodeSpeed: number | null;
};

// NetworkInput は、刻みごとに渡す、測った値。
export type NetworkInput = {
  // active は、判定してよいか。再生していて (一時停止、seek、終わりでなく)、タブが見えているとき。
  active: boolean;
  // waiting は、waiting から playing までの間か。自分で起こした詰まり (飛び直し、低遅延モードの飛び) は除いて渡す。
  waiting: boolean;
  // lowReadyState は、readyState が HAVE_FUTURE_DATA 未満か。
  lowReadyState: boolean;
  // frameAgeMs は、最後にコマが映ってからの時間。測らないとき (音声だけ、requestVideoFrameCallback が無い) は null。
  frameAgeMs: number | null;
  // ahead は、今の位置から先に読み込んである量 (秒)。
  ahead: number;
  // broadcastLag は、実時間 (秒) − 届いている映像の端のメディア時刻 (ライブ)。起点は任意で、伸びだけを見る。分からなければ null。
  broadcastLag: number | null;
  // receiveRate は、受け取りの速さ (bps)。分からない (自分で読むのを絞っている) なら null。
  receiveRate: number | null;
  server: ServerReport | null;
};

// NetworkContext は、刻みごとに渡す、段の表と今の段。
export type NetworkContext = {
  // ladder は、動かしてよい段を高い順に並べたもの。端末のデコードが追いつかなかった段より上と、自動で選ぶ経路より上は除く。
  // 音声だけの段を使ってよければ、最後に audioStep を置く。
  ladder: readonly Step[];
  // current は、今の段の名前。
  current: string;
  // automatic は、段を動かしてよいか (画質が「自動」か)。手で選んでいれば、判定だけをして動かさない。
  automatic: boolean;
  mode: NetworkMode;
};

// Signal は、「足りない」の兆し。
export type Signal = 'stall' | 'frameGap' | 'drain' | 'lagGrowth' | 'backlog' | 'receiveRate';

// StepReason は、段を動かした理由。bandwidth は帯域の不足、recovered は帯域の回復、upFailed は上げた段が持たなかったこと。
export type StepReason = 'bandwidth' | 'recovered' | 'upFailed';

// NetworkDecision は、刻みで決めたこと。step は段を to へ動かす (呼んだ側がすぐに切り替える)。probe は bytes の試しのデータを取る。
export type NetworkDecision =
  | { kind: 'none' }
  | { kind: 'step'; to: string; direction: 'down' | 'up'; reason: StepReason }
  | { kind: 'probe'; bytes: number };

// ProbeOutcome は、試しのデータを取った結果。measured は速さを測れた。failed は 5xx か打ち切り (足りない兆しが出た) で、
// 余裕が無かったと測ったことにする。rateLimited は 429 で、retryAfterMs (無ければ null) の間は測らない。
export type ProbeOutcome =
  | { kind: 'measured'; bps: number }
  | { kind: 'failed' }
  | { kind: 'rateLimited'; retryAfterMs: number | null };

// NetworkStatus は、再生の情報に出す様子。
export type NetworkStatus = {
  // signals は、最後の刻みで立っていた兆し。
  signals: Signal[];
  // receiveRate は、最後の刻みで受け取った速さ (bps)。
  receiveRate: number | null;
  // probe は、最後に試しで測った速さ (bps) と、その時刻。
  probe: { bps: number; at: number } | null;
  // nextProbeIn は、次に試すまでの時間 (ミリ秒)。試す経路でなければ null。
  nextProbeIn: number | null;
  // encoderLagging は、サーバー側の変換が間に合っていないとみなしたか。
  encoderLagging: boolean;
  // steps は、回線のために段を動かした回数。
  steps: number;
};

// NetworkController は、回線の側の判定と、段の上げ下げを持つ。時刻 now は、どれもミリ秒。
export type NetworkController = {
  // tick は、刻みごとに呼び、決めたことを返す。
  tick(now: number, input: NetworkInput, context: NetworkContext): NetworkDecision;
  // restart は、選局、シーク、経路の種類の切り替え (ライブと追っかけ) を知らせる。しばらく数えず、溜めた兆しを捨てる。
  restart(now: number): void;
  // switched は、段が変わったことを知らせる。自分で決めた切り替えのほか、手で選び直したときも呼ぶ。
  switched(now: number, current: string): void;
  // shown は、切り替えた段の最初のコマ (音声だけなら再生の始まり) が出たことを知らせる。
  shown(now: number): void;
  // probed は、試しのデータを取った結果を知らせる。
  probed(now: number, outcome: ProbeOutcome): void;
  // noSlot は、サーバが枠の不足で段を下げられなかったことを知らせる。
  noSlot(now: number): void;
  // short は、最後の刻みで「足りない」とみなしたか。手で選んだ画質で、「自動」を勧めるのに使う。
  short(): boolean;
  // probing は、試しのデータを取っている最中か。
  probing(): boolean;
  status(now: number): NetworkStatus;
};

// maxBackoff は、待ちの倍率の上限。待ちそのものは待ちの上限で抑えるので、数が大きくなりすぎないためだけに置く。
const maxBackoff = 1_024;

type TimedValue = { at: number; value: number };
type TimedFlag = { at: number; short: boolean };

// PendingUp は、上げた段の見張り。from は上げる前の段。
type PendingUp = { from: string; to: string; requestedAt: number; shownAt: number | null };

// stepIndex は、段の表の中の name の位置。無ければ -1。
function stepIndex(ladder: readonly Step[], name: string): number {
  return ladder.findIndex((step) => step.name === name);
}

// videoSteps は、段の表のうち、映像のある段。
function videoSteps(ladder: readonly Step[]): Step[] {
  return ladder.filter((step) => step.name !== audioStep);
}

// probeBytes は、次の段 need (bps) を試すのに取る量。(need × 試しの余裕の倍率 − 今の受け取りの速さ) で、試しの量の時間かかる量にし、
// 下限と上限に丸める。
export function probeBytes(need: number, receiveRate: number | null, params: NetworkParams): number {
  const bits = (need * params.probeHeadroom - (receiveRate ?? 0)) * params.probeSeconds;

  return Math.round(Math.min(Math.max(bits / 8, params.probeMinBytes), params.probeMaxBytes));
}

// chooseDown は、回線のために下げる先を返す。速さが分かれば、受け取りの速さ × 余白の係数に収まる最も高い段 (今より下)。
// 収まる段が無ければ、映像のある最も低い段。速さが分からなければ、決めた段数だけ下げる。下げる段が無ければ null。
// 音声だけの段へは、ここでは下げない (最も低い段で足りない状態が続いたときだけ、別に決める)。
export function chooseDown(
  ladder: readonly Step[],
  current: string,
  receiveRate: number | null,
  need: (step: Step, index: number) => number,
  params: NetworkParams,
): string | null {
  const steps = videoSteps(ladder);
  const index = stepIndex(steps, current);
  const lowest = steps.length - 1;
  if (index < 0 || index >= lowest) {
    return null;
  }
  if (receiveRate === null) {
    return steps[Math.min(index + params.unknownRateSteps, lowest)]!.name;
  }
  const budget = receiveRate * params.margin;
  for (let i = index + 1; i <= lowest; i++) {
    if (need(steps[i]!, i) <= budget) {
      return steps[i]!.name;
    }
  }

  return steps[lowest]!.name;
}

export type NetworkOptions = { now: number; params?: NetworkParams };

// createNetworkController は、回線の制御を作る。
export function createNetworkController(options: NetworkOptions): NetworkController {
  const params = options.params ?? defaultNetworkParams;
  let ignoreUntil = options.now + params.ignoreAfterSwitchMs;
  let holdUntil = -Infinity;
  let noSlotUntil = -Infinity;
  // arrivedAt は、今の段に移った時刻。音声だけを考えるまでの時間に使う。
  let arrivedAt = options.now;
  let current: string | null = null;
  let lowSince: number | null = null;
  let aheads: TimedValue[] = [];
  let lags: TimedValue[] = [];
  let backlogs: TimedValue[] = [];
  let rateShortSince: number | null = null;
  let encodeSlowSince: number | null = null;
  let flags: TimedFlag[] = [];
  // okSince は、足りない兆しが無い状態が始まった時刻。判定していない間 (一時停止など) は null。
  let okSince: number | null = null;
  let rateHeadroomSince: number | null = null;
  // backoff は、次に上げるまでの待ちに掛ける倍率。上げた段が持たなければ増やし、持てば 1 に戻す。
  let backoff = 1;
  let nextProbeAt = -Infinity;
  let inFlight = false;
  let lastProbe: { bps: number; at: number } | null = null;
  let pending: PendingUp | null = null;
  // sufficient は、段ごとの、足りていた間の受け取りの速さ。最高画質へ戻すときに比べる。
  const sufficient = new Map<string, number>();
  let lastSignals: Signal[] = [];
  let lastRate: number | null = null;
  let lastMode: NetworkMode | null = null;
  let encoderLagging = false;
  let steps = 0;

  const forgetHistory = () => {
    lowSince = null;
    aheads = [];
    lags = [];
    backlogs = [];
    rateShortSince = null;
    rateHeadroomSince = null;
  };

  // need は、段を流すのに要る速さ。最高画質は、足りていた間に測った受け取りの速さ、無ければその段のビットレート、
  // それも分からなければ既定値。
  const need = (step: Step, index: number): number =>
    (index === 0 ? sufficient.get(step.name) : undefined) ??
    step.bitrate ??
    params.defaultTopBitrate;

  // draining は、バッファが見る量より少なく、減少の継続時間より長く減り続け、その間の平均の減り方が閾値以上か。
  // 塊が届くたびに少し増えるので、窓の初めの量を一度も超えずに減ったことを「減り続け」とみなす。
  const draining = (now: number, ahead: number): boolean => {
    aheads.push({ at: now, value: ahead });
    while (aheads.length > 1 && now - aheads[1]!.at >= params.drainDurationMs) {
      aheads.shift();
    }
    const first = aheads[0]!;
    const span = now - first.at;
    if (ahead >= params.drainBelow || span < params.drainDurationMs) {
      return false;
    }
    if (aheads.slice(1).some((sample) => sample.value > first.value)) {
      return false;
    }

    return (first.value - ahead) / (span / 1_000) >= params.drainRate;
  };

  // lagGrowing は、伸びを見る窓の、最初の区間の最小値と最後の区間の最小値を比べ、伸びの閾値以上伸びたか。
  // 1 回の刻みで端が戻ったとみなす伸び以上伸びたら、別のストリームになったとみなして測り直す。
  const lagGrowing = (now: number, lag: number): boolean => {
    const last = lags.at(-1);
    if (last !== undefined && lag - last.value >= params.lagResetGrowth) {
      lags = [];
    }
    lags.push({ at: now, value: lag });
    while (lags.length > 1 && now - lags[1]!.at >= params.lagWindowMs) {
      lags.shift();
    }
    const start = lags[0]!.at;
    if (now - start < params.lagWindowMs) {
      return false;
    }
    const minIn = (from: number, to: number) =>
      Math.min(...lags.filter((s) => s.at >= from && s.at <= to).map((s) => s.value));

    return (
      minIn(now - params.lagEdgeMs, now) - minIn(start, start + params.lagEdgeMs) >=
      params.lagGrowth
    );
  };

  // backlogGrowing は、送れずに溜まっている量が、バックログの継続時間のあいだ伸び続けたか。
  const backlogGrowing = (now: number, backlog: number | null): boolean => {
    if (backlog === null) {
      backlogs = [];

      return false;
    }
    const last = backlogs.at(-1);
    if (last !== undefined && backlog <= last.value) {
      backlogs = [];
    }
    backlogs.push({ at: now, value: backlog });

    return now - backlogs[0]!.at >= params.backlogDurationMs;
  };

  // judge は、今の刻みで立っている兆しを返す。
  const judge = (now: number, input: NetworkInput, context: NetworkContext): Signal[] => {
    const signals: Signal[] = [];
    const speed = input.server?.encodeSpeed ?? null;
    // 速さが 0 なら測れていない (映像として読めないトラック) とみなし、遅れと読まない。
    const encodeSlow = speed !== null && speed > 0 && speed < params.encodeSlowSpeed;
    encodeSlowSince = encodeSlow ? (encodeSlowSince ?? now) : null;
    encoderLagging =
      encodeSlowSince !== null && now - encodeSlowSince >= params.encodeSlowDurationMs;
    lowSince = input.lowReadyState ? (lowSince ?? now) : null;
    // サーバのエンコードが遅れている間は、詰まりもバッファの減少も回線のせいにしない。
    if (
      !encodeSlow &&
      (input.waiting || (lowSince !== null && now - lowSince >= params.stallMs))
    ) {
      signals.push('stall');
    }
    if (input.frameAgeMs !== null && input.frameAgeMs >= params.frameGapMs) {
      signals.push('frameGap');
    }
    if (draining(now, input.ahead) && !encodeSlow) {
      signals.push('drain');
    }
    if (
      context.mode === 'probe' &&
      input.broadcastLag !== null &&
      lagGrowing(now, input.broadcastLag)
    ) {
      signals.push('lagGrowth');
    }
    if (
      input.server !== null &&
      (input.server.dropped || backlogGrowing(now, input.server.backlogBytes))
    ) {
      signals.push('backlog');
    }
    if (context.mode === 'rate') {
      const index = stepIndex(context.ladder, context.current);
      const step = context.ladder[index];
      const lacking =
        step !== undefined &&
        input.receiveRate !== null &&
        input.receiveRate * params.margin < need(step, index);
      rateShortSince = lacking ? (rateShortSince ?? now) : null;
      if (
        rateShortSince !== null &&
        now - rateShortSince >= params.rateWindowMs * params.rateShortWindows
      ) {
        signals.push('receiveRate');
      }
    }

    return signals;
  };

  // shortRatio は、直近の音声だけを判定する窓のうち、「足りない」だった刻みの割合。窓が埋まっていなければ 0。
  const shortRatio = (now: number): number => {
    flags = flags.filter((flag) => now - flag.at < params.audioWindowMs);
    const first = flags[0];
    if (first === undefined || now - first.at < params.audioWindowMs - params.tickMs * 2) {
      return 0;
    }

    return flags.filter((flag) => flag.short).length / flags.length;
  };

  // upWait は、上げを試すまでの待ち (base) に、待ちの倍率を掛けたもの。待ちの上限を超えない。
  const upWait = (base: number): number => Math.min(base * backoff, params.backoffMaxMs);

  // step は、段を to へ動かすと決め、切り替え後の据え置きを始める。
  const step = (
    now: number,
    from: string,
    to: string,
    direction: 'down' | 'up',
    reason: StepReason,
  ): NetworkDecision => {
    steps += 1;
    pending = direction === 'up' ? { from, to, requestedAt: now, shownAt: null } : null;
    current = to;
    arrivedAt = now;
    holdUntil = now + params.holdAfterSwitchMs;
    ignoreUntil = now + params.ignoreAfterSwitchMs;
    okSince = null;
    lastProbe = null;
    forgetHistory();
    flags = [];

    return { kind: 'step', to, direction, reason };
  };

  // failUp は、上げた段が持たなかったので、上げる前の段へ戻し、次に上げるまでの待ちを延ばす。
  const failUp = (now: number, up: PendingUp): NetworkDecision => {
    backoff = Math.min(backoff * params.backoffMultiplier, maxBackoff);

    return step(now, up.to, up.from, 'down', 'upFailed');
  };

  return {
    tick(now, input, context) {
      if (context.mode !== lastMode) {
        // 経路の種類が変わったら (ライブと追っかけ)、溜めた兆しを捨てる。
        if (lastMode !== null) {
          ignoreUntil = Math.max(ignoreUntil, now + params.ignoreAfterSwitchMs);
          forgetHistory();
          okSince = null;
        }
        lastMode = context.mode;
      }
      if (current === null) {
        current = context.current;
      } else if (current !== context.current) {
        // 手で選び直したなど、ここで決めていない切り替え。
        current = context.current;
        arrivedAt = now;
        ignoreUntil = Math.max(ignoreUntil, now + params.ignoreAfterSwitchMs);
        forgetHistory();
        okSince = null;
        pending = null;
      }
      lastRate = input.receiveRate;
      if (!input.active || now < ignoreUntil) {
        lastSignals = [];
        okSince = null;
        lowSince = null;
        if (!input.active) {
          // 止めていた間を挟んで、減り方や伸び方を比べない。
          forgetHistory();
        }

        return { kind: 'none' };
      }
      const signals = judge(now, input, context);
      lastSignals = signals;
      const short = signals.length > 0;
      flags.push({ at: now, short });
      const index = stepIndex(context.ladder, context.current);
      if (short) {
        okSince = null;
        rateHeadroomSince = null;
      } else {
        okSince ??= now;
        if (index >= 0 && input.receiveRate !== null) {
          sufficient.set(context.current, input.receiveRate);
        }
      }
      if (!context.automatic) {
        return { kind: 'none' };
      }

      // 上げた段が映り出さないか、見張り時間のうちに足りなくなったら、元の段へ戻す。
      if (pending !== null) {
        const up = pending;
        if (up.shownAt === null && now - up.requestedAt >= params.upToDisplayMs) {
          return failUp(now, up);
        }
        if (up.shownAt !== null && now - up.shownAt >= params.postUpWatchMs) {
          backoff = 1;
          pending = null;
        } else if (short && now >= holdUntil) {
          return failUp(now, up);
        }
      }
      if (now < holdUntil) {
        return { kind: 'none' };
      }

      if (short) {
        if (encoderLagging || now < noSlotUntil) {
          return { kind: 'none' };
        }
        const down = chooseDown(context.ladder, context.current, input.receiveRate, need, params);
        if (down !== null) {
          return step(now, context.current, down, 'down', 'bandwidth');
        }
        // 最も低い段でも、直近の窓の決めた割合以上が「足りない」なら、音声だけにする。
        const lowest = videoSteps(context.ladder).at(-1);
        if (
          lowest?.name === context.current &&
          context.ladder.at(-1)?.name === audioStep &&
          now - arrivedAt >= params.audioConsiderMs &&
          shortRatio(now) >= params.audioRatio
        ) {
          return step(now, context.current, audioStep, 'down', 'bandwidth');
        }

        return { kind: 'none' };
      }

      // 上げる。上げた段の見張りの間と、上の段が無いときは上げない。
      if (pending !== null || index <= 0) {
        return { kind: 'none' };
      }
      const upper = context.ladder[index - 1]!;
      const needUpper = need(upper, index - 1);
      if (context.mode === 'rate') {
        const enough =
          input.receiveRate !== null && input.receiveRate >= needUpper * params.upHeadroom;
        rateHeadroomSince = enough ? (rateHeadroomSince ?? now) : null;
        if (rateHeadroomSince !== null && now - rateHeadroomSince >= upWait(params.upDurationMs)) {
          return step(now, context.current, upper.name, 'up', 'recovered');
        }

        return { kind: 'none' };
      }

      if (inFlight || okSince === null) {
        return { kind: 'none' };
      }
      if (lastProbe !== null && now - lastProbe.at < params.probeValidityMs) {
        const measured = lastProbe.bps;
        lastProbe = null;
        if (measured + (input.receiveRate ?? 0) >= needUpper * params.probeHeadroom) {
          return step(now, context.current, upper.name, 'up', 'recovered');
        }
        // 足りなければ上げず、もう一度、試すまでの無事の時間を待つ。この待ちには、待ちの倍率を掛けない。
        nextProbeAt = now + params.probeAfterOkMs;

        return { kind: 'none' };
      }
      if (now - okSince < upWait(params.probeAfterOkMs) || now < nextProbeAt) {
        return { kind: 'none' };
      }
      inFlight = true;

      return { kind: 'probe', bytes: probeBytes(needUpper, input.receiveRate, params) };
    },
    restart(now) {
      ignoreUntil = Math.max(ignoreUntil, now + params.ignoreAfterSwitchMs);
      forgetHistory();
    },
    switched(now, name) {
      if (current === name) {
        return;
      }
      current = name;
      arrivedAt = now;
      ignoreUntil = Math.max(ignoreUntil, now + params.ignoreAfterSwitchMs);
      forgetHistory();
      okSince = null;
      pending = null;
    },
    shown(now) {
      if (pending !== null && pending.shownAt === null) {
        pending.shownAt = now;
      }
    },
    probed(now, outcome) {
      inFlight = false;
      switch (outcome.kind) {
        case 'measured':
          lastProbe = { bps: outcome.bps, at: now };
          break;
        case 'failed':
          // 測っていないことにすると、刻みごとに取りに行くので、余裕が無かったと測ったことにする。
          lastProbe = { bps: 0, at: now };
          break;
        case 'rateLimited':
          nextProbeAt = now + (outcome.retryAfterMs ?? params.probeAfterOkMs);
          break;
      }
    },
    noSlot(now) {
      noSlotUntil = now + params.holdAfterNoSlotMs;
    },
    short() {
      return lastSignals.length > 0;
    },
    probing() {
      return inFlight;
    },
    status(now) {
      const nextProbeIn =
        lastMode === 'probe' && okSince !== null
          ? Math.max(okSince + upWait(params.probeAfterOkMs), nextProbeAt) - now
          : null;

      return {
        signals: lastSignals,
        receiveRate: lastRate,
        probe: lastProbe,
        nextProbeIn: nextProbeIn === null ? null : Math.max(nextProbeIn, 0),
        encoderLagging,
        steps,
      };
    },
  };
}
