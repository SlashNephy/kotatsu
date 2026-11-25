// ブラウザ内でデコードする経路 (#1516、設計書 7.2) で、端末のデコードが間に合っているかを判定する。
// 「判定のウィンドウ」ごとに数え、少し遅れたら B フレームのデコードを省き、悪いウィンドウが続いたら、ほかの経路 (画質の「自動」で選ぶ経路) へ移す。
// 時刻はどれも performance の時刻 (ミリ秒)。

// JudgeParams は、#1516 の「決めるパラメーター」のうち、判定に使うもの。値は設計書の付録 A。
export type JudgeParams = {
  // windowMs は、判定のウィンドウの長さ。
  windowMs: number;
  // minFrames は、ウィンドウを数えるのに要る、デコードしたコマの数。裏に回っていたウィンドウを数えないため。
  minFrames: number;
  // decodeQuantile と decodeBudget は、1 コマのデコード時間のこの分位が、1 コマの長さ × decodeBudget を超えたら悪いとみなす値。
  decodeQuantile: number;
  decodeBudget: number;
  // skipBWiden は、B フレームを省いている間に、デコード時間の許容を広げる倍率。
  skipBWiden: number;
  // badLagMs は、映像が時計からこれだけ遅れたら悪いとみなす遅れ。
  badLagMs: number;
  // badDropped と badDropRatio は、破棄したコマがこの枚数以上で、かつウィンドウのコマ数のこの割合以上なら悪いとみなす値。
  badDropped: number;
  badDropRatio: number;
  // fallbackAfter は、悪いウィンドウがこれだけ続いたら、ほかの経路へ移す回数。
  fallbackAfter: number;
  // ignoreMs は、タブが戻った直後、読み始めた直後、データが途切れて戻った直後に数えない時間。
  ignoreMs: number;
  // skipDropped と skipLagMs は、ウィンドウの中で破棄したコマがこの枚数以上か、遅れがこれ以上なら、B フレームを省き始める値。
  skipDropped: number;
  skipLagMs: number;
  // skipInitialMs は、B フレームを最初に省く長さ。過ぎたら戻してみる。
  skipInitialMs: number;
  // relapseMs は、戻してからこの時間の中でまた遅れたら、省く長さを skipMultiplier 倍にする (上限は skipMaxMs)。
  relapseMs: number;
  skipMultiplier: number;
  skipMaxMs: number;
  // audioGapMs は、映像は届いているのに音がこれだけ届かなければ、ほかの経路へ移す時間。
  audioGapMs: number;
  // audioFailures は、AAC のデコードがこれだけ続けて失敗したら、ほかの経路へ移す回数。
  audioFailures: number;
};

export const defaultJudgeParams: JudgeParams = {
  windowMs: 2_000,
  minFrames: 30,
  decodeQuantile: 0.95,
  decodeBudget: 0.7,
  skipBWiden: 3,
  badLagMs: 500,
  badDropped: 10,
  badDropRatio: 0.1,
  fallbackAfter: 3,
  ignoreMs: 3_000,
  skipDropped: 5,
  skipLagMs: 200,
  skipInitialMs: 10_000,
  relapseMs: 10_000,
  skipMultiplier: 2,
  skipMaxMs: 60_000,
  audioGapMs: 15_000,
  audioFailures: 10,
};

// FallbackReason は、ほかの経路へ移す理由。slow はデコードが間に合わない、audio は音が届かないかデコードできない。
export type FallbackReason = 'slow' | 'audio';

// Verdict は、tick が返す、worker がすること。
export type Verdict = { type: 'skipB'; on: boolean } | { type: 'fallback'; reason: FallbackReason };

// DecodeJudge は、デコードの様子を受け取り、B フレームを省くかと、ほかの経路へ移すかを決める。
export class DecodeJudge {
  private readonly params: JudgeParams;
  private windowStart: number | null = null;
  private decodeTimes: number[] = [];
  private frames = 0;
  private dropped = 0;
  private maxLagMs = 0;
  private bad = 0;
  private ignoreUntil = -Infinity;
  private starved = false;
  private skipping = false;
  private skipUntil = 0;
  private skipDuration: number | null = null;
  private revertedAt: number | null = null;
  private lastVideo: number | null = null;
  private lastAudio: number | null = null;
  private audioFailureRun = 0;
  private fallen = false;

  constructor(params: Partial<JudgeParams> = {}) {
    this.params = { ...defaultJudgeParams, ...params };
  }

  // skippingB は、B フレームのデコードを省いているか。
  get skippingB(): boolean {
    return this.skipping;
  }

  // start は、読み始めたときに呼ぶ。数えたものを破棄し、数えない時間を始める。
  start(now: number): void {
    this.windowStart = null;
    this.clearWindow();
    this.bad = 0;
    this.starved = false;
    this.skipping = false;
    this.skipDuration = null;
    this.revertedAt = null;
    this.lastVideo = null;
    this.lastAudio = now;
    this.audioFailureRun = 0;
    this.fallen = false;
    this.ignoreUntil = now + this.params.ignoreMs;
  }

  // resume は、タブが見えるようになったときと、再開したときに呼ぶ (I フレームまで飛んでデコードし直すため)。
  // それまでに数えた悪いウィンドウの数は残す。
  resume(now: number): void {
    this.restartWindow(now);
    this.ignoreUntil = now + this.params.ignoreMs;
  }

  // starve は、デコードに回すデータが来ていない (回線が詰まった) ときに呼ぶ。来るまで数えない。
  starve(): void {
    this.starved = true;
  }

  // onVideo は、映像のデータが届いたときに呼ぶ。詰まりから戻ったら、数えない時間を始める。
  onVideo(now: number): void {
    this.lastVideo = now;
    if (this.starved) {
      this.starved = false;
      this.resume(now);
    }
  }

  // onAudio は、鳴らす音声のデータが届いたときに呼ぶ。
  onAudio(now: number): void {
    this.lastAudio = now;
  }

  // onAudioDecoded は、AAC のフレームを 1 つデコードしたときに呼ぶ。ok は、デコードできたか。
  onAudioDecoded(ok: boolean): void {
    this.audioFailureRun = ok ? 0 : this.audioFailureRun + 1;
  }

  // onDecode は、映像の PES を 1 つデコードしたときに、かかった時間 (GPU へのアップロードを含む) を渡す。
  onDecode(ms: number): void {
    this.decodeTimes.push(ms);
  }

  // onPicture は、デコードしたコマを待ち行列に足したときに呼ぶ。
  onPicture(): void {
    this.frames += 1;
  }

  // onDropped は、映さずに破棄したコマの数を渡す。
  onDropped(count: number): void {
    this.dropped += count;
  }

  // onLag は、映すコマが時計からどれだけ遅れているかを渡す。
  onLag(ms: number): void {
    this.maxLagMs = Math.max(this.maxLagMs, ms);
  }

  // tick は、定期的に呼び、worker がすることを返す。frameMs は 1 コマの長さ (分からなければ null)。
  tick(now: number, frameMs: number | null): Verdict[] {
    if (this.fallen) {
      return [];
    }
    const verdicts: Verdict[] = [];
    const audio = this.audioVerdict(now);
    if (audio !== null) {
      this.fallen = true;

      return [audio];
    }
    if (this.skipping && now >= this.skipUntil) {
      this.skipping = false;
      this.revertedAt = now;
      verdicts.push({ type: 'skipB', on: false });
    }
    if (this.windowStart === null) {
      this.restartWindow(now);

      return verdicts;
    }
    if (now - this.windowStart < this.params.windowMs) {
      return verdicts;
    }
    // 数えない時間に少しでも掛かったウィンドウは数えない。
    const counted =
      !this.starved && this.windowStart >= this.ignoreUntil && this.frames >= this.params.minFrames;
    if (counted) {
      if (this.isBad(frameMs)) {
        this.bad += 1;
      } else {
        this.bad = 0;
      }
      if (this.bad >= this.params.fallbackAfter) {
        this.fallen = true;
        this.clearWindow();

        return [{ type: 'fallback', reason: 'slow' }];
      }
      if (!this.skipping && this.isLate()) {
        this.startSkipping(now);
        verdicts.push({ type: 'skipB', on: true });
      }
    }
    this.restartWindow(now);

    return verdicts;
  }

  private audioVerdict(now: number): Verdict | null {
    if (this.audioFailureRun > this.params.audioFailures) {
      return { type: 'fallback', reason: 'audio' };
    }
    // 映像が届いている (直近のウィンドウの中に届いた) のに、音が来ない。
    const videoFlowing =
      this.lastVideo !== null && now - this.lastVideo < this.params.windowMs && !this.starved;
    if (videoFlowing && this.lastAudio !== null && now - this.lastAudio >= this.params.audioGapMs) {
      return { type: 'fallback', reason: 'audio' };
    }

    return null;
  }

  private isBad(frameMs: number | null): boolean {
    const { params } = this;
    if (frameMs !== null && this.decodeTimes.length > 0) {
      const sorted = [...this.decodeTimes].sort((a, b) => a - b);
      const index = Math.min(sorted.length - 1, Math.floor(sorted.length * params.decodeQuantile));
      const widen = this.skipping ? params.skipBWiden : 1;
      if (sorted[index]! > frameMs * params.decodeBudget * widen) {
        return true;
      }
    }
    if (this.maxLagMs >= params.badLagMs) {
      return true;
    }

    return this.dropped >= params.badDropped && this.dropped >= this.frames * params.badDropRatio;
  }

  private isLate(): boolean {
    return this.dropped >= this.params.skipDropped || this.maxLagMs >= this.params.skipLagMs;
  }

  private startSkipping(now: number): void {
    const { params } = this;
    const relapsed =
      this.revertedAt !== null &&
      this.skipDuration !== null &&
      now - this.revertedAt <= params.relapseMs;
    this.skipDuration = relapsed
      ? Math.min(this.skipDuration! * params.skipMultiplier, params.skipMaxMs)
      : params.skipInitialMs;
    this.skipping = true;
    this.skipUntil = now + this.skipDuration;
  }

  private restartWindow(now: number): void {
    this.windowStart = now;
    this.clearWindow();
  }

  private clearWindow(): void {
    this.decodeTimes = [];
    this.frames = 0;
    this.dropped = 0;
    this.maxLagMs = 0;
  }
}
