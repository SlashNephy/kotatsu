// 回線の側の兆候で画質の「自動」の段階を下げ、プローブのデータや受け取りの速さで余裕を確認してから上げる (#1513、設計書 7.1、7.5)。
// video の要素にも React にも触れず、時刻と計測した値を渡されて決めるだけにする。要素を読んで渡すのは useNetworkQuality。

// NetworkParams は、制御の値。秒の値は秒、時間の値はミリ秒、速さは bps で持つ。既定値は設計書の付録 A。
export type NetworkParams = {
  // tickMs は、回線の側の兆候を見る間隔 (回線の判定の間隔)。
  tickMs: number;
  // ignoreAfterSwitchMs は、選局、シーク、段階の切り替えの直後に数えない時間 (切り替え直後の無視時間)。
  ignoreAfterSwitchMs: number;
  // stallMs は、readyState が HAVE_FUTURE_DATA 未満の状態を、詰まりとみなすまでの時間 (詰まりとみなす時間)。
  stallMs: number;
  // frameGapMs は、次のフレームが来ないのを、映像だけ止まったとみなすまでの時間 (フレームの途絶とみなす時間)。
  frameGapMs: number;
  // frameGapAheadBelow は、フレームの途絶を回線の兆候に数える、先に読み込んである量の上限 (秒、フレームの途絶を数える先読みの上限)。
  // 先読みが十分にあるのにフレームが途切れるのは、回線ではなく、変換か描画の側で途切れたとみなす (#2033)。
  frameGapAheadBelow: number;
  // drainBelow、drainDurationMs、drainRate は、バッファの減少を「足りない」とみなす、見るバッファの量 (秒)、減少の継続時間、
  // 減少の速さの閾値 (1 秒あたりの秒)。drainEdgeMs は、減少の継続時間のウィンドウの最初と最後の区間の長さ。区間の最小値どうしを比較する。
  drainBelow: number;
  drainDurationMs: number;
  drainEdgeMs: number;
  drainRate: number;
  // lagWindowMs、lagEdgeMs、lagGrowth、lagResetGrowth は、放送に対する遅れの伸びを見るウィンドウ、ウィンドウの端の区間、伸びの閾値 (秒)、
  // 端が戻ったとみなす伸び (秒)。
  lagWindowMs: number;
  lagEdgeMs: number;
  lagGrowth: number;
  lagResetGrowth: number;
  // backlogDurationMs は、サーバのバックログが伸び続けたら足りないとみなす時間 (バックログの継続時間)。
  backlogDurationMs: number;
  // margin は、受け取りの速さのうち、段階に使ってよい割合 (余白の係数)。
  margin: number;
  // rateWindowMs と rateShortWindows は、受け取りの速さのウィンドウと、その不足が続いたら下げるまでのウィンドウの数 (速さ不足の継続時間)。
  rateWindowMs: number;
  rateShortWindows: number;
  // encodeSlowSpeed と encodeSlowDurationMs は、サーバのエンコードが追いつかないとみなす、エンコードの速さの閾値と継続時間。
  encodeSlowSpeed: number;
  encodeSlowDurationMs: number;
  // unknownRateSteps は、受け取りの速さが分からないときに一度に下げる段階の数 (速さが分からないときに下げる段階数)。
  unknownRateSteps: number;
  // audioConsiderMs、audioWindowMs、audioRatio は、最も低い段階に移ってから音声だけを考えるまでの時間、音声だけを判定するウィンドウ、
  // そのウィンドウのうち「足りない」だった時間の割合 (音声だけにする割合)。
  audioConsiderMs: number;
  audioWindowMs: number;
  audioRatio: number;
  // holdAfterSwitchMs と holdAfterNoSlotMs は、切り替え後の据え置きと、枠不足のあとの据え置き。
  holdAfterSwitchMs: number;
  holdAfterNoSlotMs: number;
  // upHeadroom と upDurationMs は、受け取りの速さで上げる経路の、上げる余裕の倍率と、上げる余裕の継続時間。
  upHeadroom: number;
  upDurationMs: number;
  // probeAfterOkMs は、プローブのデータで上げる経路で、足りない兆候が無い状態がこれだけ続いたら試す時間 (試すまでの無事の時間)。
  probeAfterOkMs: number;
  // probeHeadroom、probeSeconds、probeMinBytes、probeMaxBytes、probeTimeoutMs は、プローブの余裕の倍率、プローブの量の時間、
  // プローブの量の下限と上限、プローブの打ち切り。
  probeHeadroom: number;
  probeSeconds: number;
  probeMinBytes: number;
  probeMaxBytes: number;
  probeTimeoutMs: number;
  // probeValidityMs は、プローブで計測した速さを使う期間 (計測した値の有効期間)。
  probeValidityMs: number;
  // postUpWatchMs、backoffMultiplier、backoffMaxMs は、上げ直後の見張り時間、待ちの倍率、待ちの上限。
  postUpWatchMs: number;
  backoffMultiplier: number;
  backoffMaxMs: number;
  // upToDisplayMs は、段階を上げるよう求めてから、新しい段階が映り出すのを待つ時間 (上げてから映るまでの上限)。
  upToDisplayMs: number;
  // downToDisplayMs は、段階を下げてから、新しい段階が映り出すのを待つ時間 (下げてから映るまでの上限)。過ぎたら、もう一段下げる。
  // 下げた段階も回線に収まらなければ、最初のセグメントが届かず、映り出さないまま待ち続けるため。
  downToDisplayMs: number;
  // defaultTopBitrate は、最高画質 (オリジナル) のビットレートが分からず、足りていた間の受け取りの速さも計測していないときに使う値。
  defaultTopBitrate: number;
};

// defaultNetworkParams は、既定値。どれも初期値で、実機で計測して決め直す (設計書の付録 A)。
export const defaultNetworkParams: NetworkParams = {
  tickMs: 500,
  // 低遅延モードの目標量 (初期 1 秒、上限 5 秒) を貯め、kyusu の経路で最初のセグメントが映るまでより長くする。
  ignoreAfterSwitchMs: 8_000,
  // 1 フレーム (約 0.033 秒) と、変換の経路の断片の間隔 (約 0.5 秒) より十分長くする。
  stallMs: 1_000,
  frameGapMs: 1_500,
  // 変換の経路の断片 (約 0.5 秒) の数個分より多くし、読み込みが追いつかずに途切れたときは数える。
  frameGapAheadBelow: 2.5,
  drainBelow: 4,
  // チャンクが届くたびにバッファは増え、次のチャンクまでは減る (kyusu のセグメントでは 2 秒ごと)。区間をチャンクの間隔より長くし、
  // 区間の最小値 (チャンクが届く直前の底) どうしを比較して、のこぎりの歯の形の上下を「減り続け」と読まないようにする。
  drainDurationMs: 8_000,
  drainEdgeMs: 2_500,
  // 低遅延モードで速めているときの減り方 (1.1 − 1 = 0.1) より大きくする。
  drainRate: 0.2,
  // kyusu のセグメント (2 秒) の数倍にし、端の区間をセグメントより長く、ウィンドウの半分より短くする。
  lagWindowMs: 20_000,
  lagEdgeMs: 5_000,
  lagGrowth: 1.5,
  lagResetGrowth: 5,
  backlogDurationMs: 3_000,
  margin: 0.8,
  // ウィンドウ × 数 (6 秒) を、録画の再生で先にバッファする量 (変換の経路の maxAheadSeconds、8 秒) より短くする。
  rateWindowMs: 3_000,
  rateShortWindows: 2,
  encodeSlowSpeed: 0.9,
  encodeSlowDurationMs: 5_000,
  // オリジナル (13〜24 Mbps) と kyusu の 1080p (約 6.1 Mbps) の差は、ほかの段階どうしの差 (約 1.7〜2 倍) より大きいので、
  // オリジナルからも 1 段階で足りるとみなす。
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
  downToDisplayMs: 15_000,
  // 地上波と BS の TS (13〜24 Mbps、設計書 7.1) の間。
  defaultTopBitrate: 20_000_000,
};

// audioStep は、段階のテーブルの、音声だけの段階の名前。kyusu の映像を送らない画質の名前と同じ。
export const audioStep = 'audio';
// audioOnlyBitrate は、音声だけの段階のビットレート (bps)。kyusu の音声の 128 kbps。
export const audioOnlyBitrate = 128_000;

// Step は、段階のテーブルの 1 段階。高い順に並べて渡す。bitrate は映像と音声を足した速さ (bps)。分からなければ (オリジナル) null。
export type Step = { name: string; bitrate: number | null };

// NetworkMode は、上げ方。probe はプローブのデータで (ライブ。実時間でしか届かず、受け取りの速さでは余裕が分からない)、
// rate は受け取りの速さで (録画と追っかけ。先に読むので、受け取りの速さで余裕が分かる) 確認する。
export type NetworkMode = 'probe' | 'rate';

// ServerReport は、送り手が通知する様子。今の tenban と kyusu は通知しないので、使っていない (設計書 7.5)。
export type ServerReport = {
  // backlogBytes は、送れずに溜まっている量。分からなければ null。
  backlogBytes: number | null;
  // dropped は、サーバがチャンクを破棄したか。
  dropped: boolean;
  // encodeSpeed は、エンコードの速さ (出した映像の時刻の進み ÷ 実時間)。0 は測れていない。分からなければ null。
  encodeSpeed: number | null;
};

// NetworkInput は、判定のたびに渡す、計測した値。
export type NetworkInput = {
  // active は、判定してよいか。再生していて (一時停止、seek、終わりでなく)、タブが見えているとき。
  active: boolean;
  // started は、今の段階で再生が始まったか。始まるまで (読み込みの最中) は、詰まりを数えない。
  started: boolean;
  // loadedToEnd は、録画を終えた録画など終わりのある流れを、今の位置から終わりまで読み込んであるか。読み込んである間は判定しない。
  // 読むものが残っておらず、先読みが減るのも受け取りの速さが落ちるのも、回線のせいではないため (#2110)。
  loadedToEnd: boolean;
  // waiting は、waiting から playing までの間か。自分で起こした詰まり (飛び直し、低遅延モードの飛び) は除いて渡す。
  waiting: boolean;
  // lowReadyState は、readyState が HAVE_FUTURE_DATA 未満か。
  lowReadyState: boolean;
  // frameAgeMs は、最後にフレームが映ってからの時間。計測しないとき (音声だけ、requestVideoFrameCallback が無い) は null。
  frameAgeMs: number | null;
  // ahead は、今の位置から先に読み込んである量 (秒)。
  ahead: number;
  // broadcastLag は、実時間 (秒) − 届いている映像の端のメディア時刻 (ライブ)。起点は任意で、伸びだけを見る。分からなければ null。
  broadcastLag: number | null;
  // receiveRate は、受け取りの速さ (bps)。分からない (自分で読むのを絞っている) なら null。
  receiveRate: number | null;
  server: ServerReport | null;
};

// NetworkContext は、判定のたびに渡す、段階のテーブルと今の段階。
export type NetworkContext = {
  // ladder は、動かしてよい段階を高い順に並べたもの。端末のデコードが追いつかなかった段階より上と、自動で選ぶ経路より上は除く。
  // 音声だけの段階を使ってよければ、最後に audioStep を置く。
  ladder: readonly Step[];
  // current は、今の段階の名前。
  current: string;
  // automatic は、段階を動かしてよいか (画質が「自動」か)。手で選んでいれば、判定だけをして動かさない。
  automatic: boolean;
  mode: NetworkMode;
};

// Signal は、「足りない」の兆候。
export type Signal = 'stall' | 'frameGap' | 'drain' | 'lagGrowth' | 'backlog' | 'receiveRate';

// StepReason は、段階を動かした理由。bandwidth は帯域の不足、recovered は帯域の回復、upFailed は上げた段階が持たなかったこと。
export type StepReason = 'bandwidth' | 'recovered' | 'upFailed';

// NetworkDecision は、判定で決めたこと。step は段階を to へ動かす (呼んだ側がすぐに切り替える)。probe は bytes のプローブのデータを取る。
export type NetworkDecision =
  | { kind: 'none' }
  | { kind: 'step'; to: string; direction: 'down' | 'up'; reason: StepReason }
  | { kind: 'probe'; bytes: number };

// ProbeOutcome は、プローブのデータを取った結果。measured は速さを測れた。failed は 5xx か打ち切り (足りない兆候が出た) で、
// 余裕が無かったと計測したことにする。rateLimited は 429 で、retryAfterMs (無ければ null) の間は計測しない。
// alongsideBps は、プローブの間に、再生中の映像が受け取った速さ (無ければ 0)。試しと映像は同じ回線を分け合うので、回線の速さは 2 つの和になる。
export type ProbeOutcome =
  | { kind: 'measured'; bps: number; alongsideBps?: number }
  | { kind: 'failed' }
  | { kind: 'rateLimited'; retryAfterMs: number | null };

// NetworkStatus は、再生の情報に出す様子。
export type NetworkStatus = {
  // signals は、最後の判定で立っていた兆候。
  signals: Signal[];
  // receiveRate は、最後の判定で受け取った速さ (bps)。
  receiveRate: number | null;
  // probe は、最後にプローブで計測した速さ (bps) と、計測してからの時間 (ミリ秒)。
  probe: { bps: number; ageMs: number } | null;
  // nextProbeIn は、次に試すまでの時間 (ミリ秒)。試す経路でなければ null。
  nextProbeIn: number | null;
  // encoderLagging は、サーバー側の変換が間に合っていないとみなしたか。
  encoderLagging: boolean;
  // steps は、回線のために段階を動かした回数。
  steps: number;
  // lastStep は、最後に段階を動かした先と、そのときに立っていた兆候、動かしてからの時間 (ミリ秒)。
  lastStep: { to: string; signals: Signal[]; ageMs: number } | null;
};

// NetworkController は、回線の側の判定と、段階の上げ下げを持つ。時刻 now は、どれもミリ秒。
export type NetworkController = {
  // tick は、判定のたびに呼び、決めたことを返す。
  tick(now: number, input: NetworkInput, context: NetworkContext): NetworkDecision;
  // restart は、選局、シーク、経路の種類の切り替え (ライブと追っかけ) を通知する。しばらく数えず、蓄積した兆候を破棄する。
  restart(now: number): void;
  // switched は、段階が変わったことを通知する。自分で決めた切り替えのほか、手で選び直したときも呼ぶ。
  switched(now: number, current: string): void;
  // shown は、切り替えた段階の最初のフレーム (音声だけなら再生の始まり) が出たことを通知する。
  shown(now: number): void;
  // probed は、プローブのデータを取った結果を通知する。
  probed(now: number, outcome: ProbeOutcome): void;
  // noSlot は、サーバが枠の不足で段階を下げられなかったことを通知する。
  noSlot(now: number): void;
  // short は、最後の判定で「足りない」とみなしたか。手で選んだ画質で、「自動」を勧めるのに使う。
  short(): boolean;
  // probing は、プローブのデータを取っている最中か。
  probing(): boolean;
  status(now: number): NetworkStatus;
};

// maxBackoff は、待ちの倍率の上限。待ちそのものは待ちの上限で抑えるので、数が大きくなりすぎないためだけに置く。
const maxBackoff = 1_024;

type TimedValue = { at: number; value: number };
type TimedFlag = { at: number; short: boolean };

// PendingUp は、上げた段階の見張り。from は上げる前の段階。
type PendingUp = { from: string; to: string; requestedAt: number; shownAt: number | null };

// stepIndex は、段階のテーブルの中の name の位置。無ければ -1。
function stepIndex(ladder: readonly Step[], name: string): number {
  return ladder.findIndex((step) => step.name === name);
}

// videoSteps は、段階のテーブルのうち、映像のある段階。
function videoSteps(ladder: readonly Step[]): Step[] {
  return ladder.filter((step) => step.name !== audioStep);
}

// probeBytes は、次の段階 need (bps) を試すのに取る量。(need × プローブの余裕の倍率 − 今の受け取りの速さ) で、プローブの量の時間かかる量にし、
// 下限と上限に丸める。
export function probeBytes(
  need: number,
  receiveRate: number | null,
  params: NetworkParams,
): number {
  const bits = (need * params.probeHeadroom - (receiveRate ?? 0)) * params.probeSeconds;

  return Math.round(Math.min(Math.max(bits / 8, params.probeMinBytes), params.probeMaxBytes));
}

// chooseDown は、回線のために下げる先を返す。速さが分かれば、受け取りの速さ × 余白の係数に収まる最も高い段階 (今より下)。
// 収まる段階が無ければ、映像のある最も低い段階。速さが分からなければ、決めた段階数だけ下げる。下げる段階が無ければ null。
// 音声だけの段階へは、ここでは下げない (最も低い段階で足りない状態が続いたときだけ、別に決める)。
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
  // arrivedAt は、今の段階に移った時刻。音声だけを考えるまでの時間に使う。
  let arrivedAt = options.now;
  let current: string | null = null;
  let lowSince: number | null = null;
  let aheads: TimedValue[] = [];
  let lags: TimedValue[] = [];
  let backlogs: TimedValue[] = [];
  let rateShortSince: number | null = null;
  let encodeSlowSince: number | null = null;
  let flags: TimedFlag[] = [];
  // okSince は、足りない兆候が無い状態が始まった時刻。判定していない間 (一時停止など) は null。
  let okSince: number | null = null;
  let rateHeadroomSince: number | null = null;
  // backoff は、次に上げるまでの待ちに掛ける倍率。上げた段階が持たなければ増やし、持てば 1 に戻す。
  let backoff = 1;
  let nextProbeAt = -Infinity;
  let inFlight = false;
  let lastProbe: { bps: number; at: number } | null = null;
  // measuredProbe は、再生の情報に出す、最後にプローブで計測した速さ。上げるかを決めても破棄しない。
  let measuredProbe: { bps: number; at: number } | null = null;
  let pending: PendingUp | null = null;
  // downAt は、段階を下げて、まだ新しい段階が映り出していない間の、下げた時刻。
  let downAt: number | null = null;
  // probeAhead は、最後の判定で、プローブのデータで上げる段階が上にあったか。無ければ、次に試すまでの時間を出さない。
  let probeAhead = false;
  // sufficient は、段階ごとの、足りていた間の受け取りの速さ。最高画質へ戻すときに比較する。
  const sufficient = new Map<string, number>();
  let lastSignals: Signal[] = [];
  let lastRate: number | null = null;
  let lastMode: NetworkMode | null = null;
  let encoderLagging = false;
  let steps = 0;
  let lastStep: { to: string; signals: Signal[]; at: number } | null = null;

  const forgetHistory = () => {
    lowSince = null;
    aheads = [];
    lags = [];
    backlogs = [];
    rateShortSince = null;
    rateHeadroomSince = null;
  };

  // need は、段階を流すのに要る速さ。最高画質は、足りていた間に計測した受け取りの速さ、無ければその段階のビットレート、
  // それも分からなければ既定値。
  const need = (step: Step, index: number): number =>
    (index === 0 ? sufficient.get(step.name) : undefined) ??
    step.bitrate ??
    params.defaultTopBitrate;

  // draining は、バッファが見る量より少なく、減少の継続時間のあいだ減り続け、その間の平均の減り方が閾値以上か。
  // チャンクが届くたびに増えるので、ウィンドウの最初と最後の区間の、最小値 (底) どうしと最大値 (山) どうしを比較し、どちらも下がったことを
  // 「減り続け」とみなす。減り方は、底どうしの差を、区間の始まりの差で割る。
  const draining = (now: number, ahead: number): boolean => {
    aheads.push({ at: now, value: ahead });
    while (aheads.length > 1 && now - aheads[1]!.at >= params.drainDurationMs) {
      aheads.shift();
    }
    const start = aheads[0]!.at;
    const span = now - start;
    if (ahead >= params.drainBelow || span < params.drainDurationMs - params.tickMs) {
      return false;
    }
    const firstSection = aheads
      .filter((s) => s.at <= start + params.drainEdgeMs)
      .map((s) => s.value);
    const lastSection = aheads.filter((s) => s.at >= now - params.drainEdgeMs).map((s) => s.value);
    if (Math.max(...lastSection) >= Math.max(...firstSection)) {
      return false;
    }
    const fall = Math.min(...firstSection) - Math.min(...lastSection);

    return fall / ((span - params.drainEdgeMs) / 1_000) >= params.drainRate;
  };

  // lagGrowing は、伸びを見るウィンドウの、最初の区間の最小値と最後の区間の最小値を比較し、伸びの閾値以上伸びたか。
  // 1 回の判定で端が戻ったとみなす伸び以上伸びたら、別のストリームになったとみなして計測し直す。
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

  // judge は、今の判定で立っている兆候を返す。
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
    if (!encodeSlow && (input.waiting || (lowSince !== null && now - lowSince >= params.stallMs))) {
      signals.push('stall');
    }
    if (
      input.frameAgeMs !== null &&
      input.frameAgeMs >= params.frameGapMs &&
      input.ahead < params.frameGapAheadBelow
    ) {
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

  // shortRatio は、直近の音声だけを判定するウィンドウのうち、「足りない」だった判定の割合。ウィンドウが埋まっていなければ 0。
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

  // step は、段階を to へ動かすと決め、切り替え後の据え置きを始める。
  const step = (
    now: number,
    from: string,
    to: string,
    direction: 'down' | 'up',
    reason: StepReason,
  ): NetworkDecision => {
    steps += 1;
    lastStep = { to, signals: lastSignals, at: now };
    pending = direction === 'up' ? { from, to, requestedAt: now, shownAt: null } : null;
    downAt = direction === 'down' ? now : null;
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

  // failUp は、上げた段階が持たなかったので、上げる前の段階へ戻し、次に上げるまでの待ちを延ばす。
  const failUp = (now: number, up: PendingUp): NetworkDecision => {
    backoff = Math.min(backoff * params.backoffMultiplier, maxBackoff);

    return step(now, up.to, up.from, 'down', 'upFailed');
  };

  return {
    tick(now, input, context) {
      probeAhead = false;
      if (context.mode !== lastMode) {
        // 経路の種類が変わったら (ライブと追っかけ)、蓄積した兆候を破棄する。前の経路で上げ下げした段階の見張りも破棄する。
        if (lastMode !== null) {
          ignoreUntil = Math.max(ignoreUntil, now + params.ignoreAfterSwitchMs);
          forgetHistory();
          okSince = null;
          pending = null;
          downAt = null;
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
        downAt = null;
      }
      lastRate = input.receiveRate;
      // 上げた段階が、上げてから映るまでの上限がたっても映り出さなければ、元の段階へ戻す。足りない段階では、最初のキーフレームが届かず、
      // 再生が始まらないことがあるので、始まる前から見る。一時停止している間は見ない。
      if (
        input.active &&
        context.automatic &&
        pending !== null &&
        pending.shownAt === null &&
        now - pending.requestedAt >= params.upToDisplayMs
      ) {
        return failUp(now, pending);
      }
      // 下げた段階が、下げてから映るまでの上限がたっても映り出さなければ、受け取りの速さに収まる段階か、最も低い段階なら音声だけへ下げる。
      if (input.started) {
        downAt = null;
      } else if (
        input.active &&
        context.automatic &&
        downAt !== null &&
        now - downAt >= params.downToDisplayMs
      ) {
        // 受け取りの速さが分からなければ (新しい段階のセグメントが 1 つも届いていない)、1 段階ずつではなく最も低い段階へ下げる。
        // 1 段階ずつ待つと、オリジナルから最も低い段階まで 1 分ほど映らないため。
        const lowest = videoSteps(context.ladder).at(-1)?.name;
        const lower =
          lowest !== undefined && context.current !== lowest && context.current !== audioStep
            ? input.receiveRate === null
              ? lowest
              : chooseDown(context.ladder, context.current, input.receiveRate, need, params)
            : context.ladder.at(-1)?.name === audioStep && context.current === lowest
              ? audioStep
              : null;
        if (lower !== null) {
          return step(now, context.current, lower, 'down', 'bandwidth');
        }
        downAt = null;
      }
      if (!input.active || !input.started || input.loadedToEnd || now < ignoreUntil) {
        lastSignals = [];
        okSince = null;
        lowSince = null;
        if (!input.active || input.loadedToEnd) {
          // 止めていた間や、終わりまで読み込んでいた間を挟んで、減り方や伸び方を比較しない。
          forgetHistory();
        }

        return { kind: 'none' };
      }
      const signals = judge(now, input, context);
      lastSignals = signals;
      const short = signals.length > 0;
      flags.push({ at: now, short });
      const index = stepIndex(context.ladder, context.current);
      // バッファの減少だけなら、試すまでの無事を途切れさせない (下げる兆候にだけ使う)。周期的に途切れる回線 (帯域は足りている) では、
      // 途切れるたびにバッファの減少が立ち、無事が続かず、下げた段階から上げられなくなるため。
      const drainOnly = short && signals.every((signal) => signal === 'drain');
      if (short) {
        rateHeadroomSince = null;
      }
      if (short && !drainOnly) {
        okSince = null;
      } else {
        okSince ??= now;
      }
      if (!short && index >= 0 && input.receiveRate !== null) {
        sufficient.set(context.current, input.receiveRate);
      }
      if (!context.automatic) {
        return { kind: 'none' };
      }

      // 上げた段階で、見張り時間のうちに足りなくなったら、元の段階へ戻す。
      if (pending !== null) {
        const up = pending;
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
        // 最も低い段階でも、直近のウィンドウの決めた割合以上が「足りない」なら、音声だけにする。
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

      // 上げる。上げた段階の見張りの間と、上の段階が無いときは上げない。
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

      probeAhead = true;
      if (inFlight || okSince === null) {
        return { kind: 'none' };
      }
      if (lastProbe !== null && now - lastProbe.at < params.probeValidityMs) {
        // プローブの前に計測した受け取りの速さは足さない。回線が詰まっていれば、プローブの間は映像が細り、同じ帯域を二重に数えるため。
        const measured = lastProbe.bps;
        lastProbe = null;
        if (measured >= needUpper * params.probeHeadroom) {
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
          lastProbe = { bps: outcome.bps + (outcome.alongsideBps ?? 0), at: now };
          measuredProbe = { bps: outcome.bps, at: now };
          break;
        case 'failed':
          // 計測していないことにすると、判定のたびに取りに行くので、余裕が無かったと計測したことにする。
          lastProbe = { bps: 0, at: now };
          measuredProbe = lastProbe;
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
        lastMode === 'probe' && probeAhead && !inFlight && okSince !== null
          ? Math.max(okSince + upWait(params.probeAfterOkMs), nextProbeAt) - now
          : null;

      return {
        signals: lastSignals,
        receiveRate: lastRate,
        probe:
          measuredProbe === null ? null : { bps: measuredProbe.bps, ageMs: now - measuredProbe.at },
        nextProbeIn: nextProbeIn === null ? null : Math.max(nextProbeIn, 0),
        encoderLagging,
        steps,
        lastStep:
          lastStep === null
            ? null
            : { to: lastStep.to, signals: lastStep.signals, ageMs: now - lastStep.at },
      };
    },
  };
}
