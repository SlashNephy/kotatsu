import type { BenchmarkClip } from './benchmarkStream';

// 「この端末で試す」(#1771、設計書 7.5)。合成の映像を、表示の経路ごとに短く再生し、コマが間に合うかを計測する。
// 利用者の設定は書き換えず、TsPlayer に経路ごとの設定を渡す。結果は画面に出すだけで、どこにも送らない。

// BenchmarkScenario は、試す経路。
export type BenchmarkScenario = {
  id: BenchmarkScenarioID;
  clip: BenchmarkClip;
  fieldRate: boolean;
  interpolate: boolean;
};
export type BenchmarkScenarioID =
  | 'fields'
  | 'frames'
  | 'framesInterpolated'
  | 'film'
  | 'filmInterpolated';

// benchmarkScenarios は、試す経路を試す順に並べたもの。補間ありの経路は、補間なしの経路と並べて差で読めるようにする。
export const benchmarkScenarios: readonly BenchmarkScenario[] = [
  { id: 'fields', clip: 'interlaced', fieldRate: true, interpolate: false },
  { id: 'frames', clip: 'interlaced', fieldRate: false, interpolate: false },
  { id: 'framesInterpolated', clip: 'interlaced', fieldRate: false, interpolate: true },
  { id: 'film', clip: 'film', fieldRate: false, interpolate: false },
  { id: 'filmInterpolated', clip: 'film', fieldRate: false, interpolate: true },
];

// baselineOf は、補間ありの経路と比較する、補間なしの経路。
export const baselineOf: Partial<Record<BenchmarkScenarioID, BenchmarkScenarioID>> = {
  framesInterpolated: 'frames',
  filmInterpolated: 'film',
};

// warmupMs は、再生を始めてから計測し始めるまでの時間。読み込みと、プルダウンを見つけるまでを計測しない。
export const warmupMs = 3_000;
// measureMs は、1 回に計測する時間。
export const measureMs = 5_000;
// runs は、経路ごとに計測する回数。結果は中央値を出す。
export const runs = 3;
// streamSeconds は、1 回に流す映像の長さ。計測し終えるまでに映像が終わらない長さにする。
export const streamSeconds = 16;

// Counters は、ある時点での、累計の数。
export type Counters = {
  // time は、計測した時刻 (ミリ秒、performance.now)。
  time: number;
  // late は、yadif が作ったのに、間に合わず出さなかった絵の累計。
  late: number;
  // dropped は、ブラウザが復号して、出さずに破棄したコマの累計 (video の要素が数えるもの)。
  dropped: number;
};

// RunResult は、1 回の測定の結果。
export type RunResult = {
  // fps は、計測した間に出した 1 秒あたりのコマの数 (yadif が通知した値の平均。補間したコマも数える)。
  fps: number;
  // latePerSecond は、1 秒あたりの、間に合わなかった絵とコマの数 (late と dropped の和)。
  latePerSecond: number;
  // film は、計測した間に、2:3 プルダウンを見つけて 24p で出していたか。
  film: boolean;
  // fieldRateDropped は、追いつかずに、フィールドごとの表示をやめたか。
  fieldRateDropped: boolean;
};

// summarizeRun は、計測し始めと計測し終わりの累計と、その間に通知した fps から、1 回の結果を出す。
// fps を 1 つも受け取らなければ null。
export function summarizeRun(
  start: Counters,
  end: Counters,
  fpsSamples: readonly number[],
  film: boolean,
  fieldRateDropped: boolean,
): RunResult | null {
  const seconds = (end.time - start.time) / 1_000;
  if (seconds <= 0 || fpsSamples.length === 0) {
    return null;
  }
  const late = end.late - start.late + (end.dropped - start.dropped);

  return {
    fps: fpsSamples.reduce((sum, value) => sum + value, 0) / fpsSamples.length,
    latePerSecond: Math.max(late, 0) / seconds,
    film,
    fieldRateDropped,
  };
}

// median は、値の中央値。空なら null。数が偶数なら、真ん中の 2 つの平均。
export function median(values: readonly number[]): number | null {
  if (values.length === 0) {
    return null;
  }
  const sorted = [...values].sort((a, b) => a - b);
  const middle = Math.floor(sorted.length / 2);

  return sorted.length % 2 === 0 ? (sorted[middle - 1]! + sorted[middle]!) / 2 : sorted[middle]!;
}

// ScenarioResult は、経路ごとに、計測した回をまとめた結果。
export type ScenarioResult = {
  id: BenchmarkScenarioID;
  // fps と latePerSecond は、測れた回の中央値。1 回も測れなければ null。
  fps: number | null;
  latePerSecond: number | null;
  // measured は、測れた回の数。
  measured: number;
  // film は、測れた回のうち、1 回でも 24p で出していたか。
  film: boolean;
  // fieldRateDropped は、測れた回のうち、1 回でもフィールドごとの表示をやめたか。
  fieldRateDropped: boolean;
};

// summarizeScenario は、経路の各回の結果をまとめる。測れなかった回は null で渡す。
export function summarizeScenario(
  id: BenchmarkScenarioID,
  results: readonly (RunResult | null)[],
): ScenarioResult {
  const measured = results.filter((result): result is RunResult => result !== null);

  return {
    id,
    fps: median(measured.map((result) => result.fps)),
    latePerSecond: median(measured.map((result) => result.latePerSecond)),
    measured: measured.length,
    film: measured.some((result) => result.film),
    fieldRateDropped: measured.some((result) => result.fieldRateDropped),
  };
}

// smoothRatio は、なめらかとみなす、出したコマの数の割合。目指すコマの数 (フィールドごとの表示は 59.94、
// 補間は画面の書き換えの速さ) にこれを掛けた数より多く出せば、なめらかとみなす。
// 60Hz の画面で、コマごとの表示の補間は、補間の推定を総当たりで探していたバージョン (#1815 の前) では、内蔵 GPU
// (UHD Graphics 770) で 40.9〜42.6 fps、スマートフォン (Pixel 10 Pro XL) で 54.7〜54.9 fps、2 ステップで探すバージョンでは、
// どちらも 60 fps だった。その間に置く。
export const smoothRatio = 0.95;
// lateMargin は、補間ありの経路で、補間なしの経路より増えてもよい、1 秒あたりの間に合わなかったコマの数。
// 上と同じ測定で、総当たりのバージョンは 0.4 から 7.0 増え、2 ステップのバージョンは 0 から 0.4 増えた。2 ステップのバージョンがぶれで落ちないよう広めに取る。
// 総当たりのバージョンの Pixel は、ここでは落ちないが、コマの数で落ちる。
export const lateMargin = 1;
// fieldRate は、フィールドごとの表示で目指す、1 秒あたりのコマの数。
const fieldRate = 60_000 / 1_001;

// judge は、経路がなめらかに表示できたかを決める。補間なしの経路 (目指す数が放送のコマの数そのもの) と、
// 測れなかった経路は null。baseline は、補間ありの経路と比較する補間なしの経路の結果。
export function judge(
  scenario: ScenarioResult,
  baseline: ScenarioResult | undefined,
  refreshInterval: number | null,
): boolean | null {
  if (scenario.fps === null || scenario.latePerSecond === null) {
    return null;
  }
  if (scenario.id === 'fields') {
    return !scenario.fieldRateDropped && scenario.fps >= fieldRate * smoothRatio;
  }
  if (baselineOf[scenario.id] === undefined) {
    return null;
  }
  if (refreshInterval === null || baseline?.latePerSecond == null) {
    return null;
  }

  return (
    scenario.fps >= (1_000 / refreshInterval) * smoothRatio &&
    scenario.latePerSecond - baseline.latePerSecond <= lateMargin
  );
}

// EstimateTiming は、コマの補間で動きを推し量る処理にかかる時間。
export type EstimateTiming = {
  // ms は、1440×1080 の 2 コマで 1 回推し量る時間 (ミリ秒)。
  ms: number;
  // method は、計測方法。gpu は GPU の時間 (EXT_disjoint_timer_query_webgl2)。
  // readback は、まとめて回して 1 画素を読み戻すまでの時間で、GPU の時間より小さく出ることがある。
  method: 'gpu' | 'readback';
};

// BenchmarkReport は、試した結果の全体。
export type BenchmarkReport = {
  // userAgent と renderer は、試した端末。renderer は WebGL の GPU の名前で、分からなければ null。
  userAgent: string;
  renderer: string | null;
  // refreshInterval は、画面の書き換えの間隔の中央値 (ミリ秒)。測れなければ null。
  refreshInterval: number | null;
  scenarios: ScenarioResult[];
  // estimate は、補間の推定の時間。WebGL2 がなく測れなければ null。
  estimate: EstimateTiming | null;
};

// formatReport は、結果を、不具合の報告に貼れるテキストにする。label は、経路の名前を返す。
export function formatReport(
  report: BenchmarkReport,
  label: (id: BenchmarkScenarioID) => string,
): string {
  const number = (value: number | null, digits: number) =>
    value === null ? '-' : value.toFixed(digits);
  const lines = [
    `User agent: ${report.userAgent}`,
    `GPU: ${report.renderer ?? '-'}`,
    `Refresh interval: ${number(report.refreshInterval, 1)} ms`,
    `Interpolation estimate: ${
      report.estimate === null
        ? '-'
        : `${report.estimate.ms.toFixed(1)} ms (${report.estimate.method})`
    }`,
  ];
  for (const scenario of report.scenarios) {
    const smooth = judge(
      scenario,
      report.scenarios.find((other) => other.id === baselineOf[scenario.id]),
      report.refreshInterval,
    );
    const notes = [
      smooth === null ? null : smooth ? 'smooth' : 'not smooth',
      scenario.film ? 'film' : null,
      scenario.fieldRateDropped ? 'field rate dropped' : null,
      scenario.measured < runs ? `${scenario.measured}/${runs} runs` : null,
    ].filter((note) => note !== null);
    lines.push(
      `${label(scenario.id)}: ${number(scenario.fps, 1)} fps, ${number(scenario.latePerSecond, 1)} late/s${
        notes.length > 0 ? ` (${notes.join(', ')})` : ''
      }`,
    );
  }

  return lines.join('\n');
}
