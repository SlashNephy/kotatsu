import { cmLookahead, cmSectionAt, shouldAutoSkip, type CmSection } from './cmSkip';

// uncoverFrames は、飛んだあと、飛び先以上のコマがこの数だけ続いて映ったら蓋を外す、その数 (#1512)。
// requestVideoFrameCallback が来るのは、コマを描画の側へ渡した時点で、画面に出るのはその後になる。
// Windows の Chrome や Edge は、映像を DOM とは別の面に出すので、蓋を外す変更のほうが映像より先に画面へ出ることがある。
// そのため、1 コマでは外さない。実機で測っていない初期値 (設計書 付録 A)。
export const uncoverFrames = 3;

// coverTimeoutMs は、蓋を出してから、外す条件を満たさなくても外すまでの時間 (#1512)。外し忘れて映像が止まって見えるのを防ぐ。
// 読み直す経路で、読み直しを求めてから飛び先のコマが映るまでの時間より長くする (設計書 付録 A)。
export const coverTimeoutMs = 5_000;

// landingTolerance は、飛び先に着いたとみなす、コマの位置の誤差 (秒)。位置の計算の丸めの分だけ許す。
const landingTolerance = 0.001;

// CoverKind は、蓋の種類。still は、映っていた本編のコマを写したもの。black は、プレイヤーの枠全体を覆う黒。
export type CoverKind = 'still' | 'black';

// SkipperFrame は、映ったコマ 1 つ分の様子。
export type SkipperFrame = {
  // position は、そのコマの再生位置 (秒)。読み直しや作り直しの途中で、まだ信じられないときは null。
  position: number | null;
  // seeking は、video が seek しているかどうか。
  seeking: boolean;
  paused: boolean;
  // presented は、コマが映ったことの知らせかどうか。timeupdate から呼んだときは false にする。
  // currentTime は、代入した時点で飛び先の値を返し、映っているコマより先へ進んでいることもあるので、蓋を外す数には入れない。
  presented: boolean;
};

// SkipperHost は、CmAutoSkipper が読む状態と、動かす先。
export type SkipperHost = {
  // enabled は、CM を自動で飛ばす設定 (#1282) かどうか。
  enabled: () => boolean;
  // scrubbing は、シークバーのつまみをつまんでいるかどうか。
  scrubbing: () => boolean;
  // lastSeek は、利用者が最後にシークした位置。自動で飛んだ位置は含めない。
  lastSeek: () => number | null;
  // sections は、続いた区間をまとめた CM の区間。
  sections: () => readonly CmSection[];
  cover: (kind: CoverKind) => void;
  uncover: () => void;
  // jump は、target へ移り、実際に移った先を返す (録画の外へは移らない)。利用者のシークとして数えない。
  jump: (target: number) => number;
  // notify は、CM を飛ばしたことを知らせる。
  notify: () => void;
  setTimer: (callback: () => void, ms: number) => () => void;
};

// CmAutoSkipper は、CM を自動で飛ばすときに、CM のコマを 1 コマも画面に出さないための、判定と蓋の出し入れ (#1512)。
// 映ったコマごとに frame を呼ぶ。いまのコマの位置に先読みの分を足して CM に入っていれば、続いた CM の終わりへ飛ぶ。
// 飛ぶ直前に蓋を出し、飛び先以上のコマが uncoverFrames だけ続いて映ってから外す。
export class CmAutoSkipper {
  readonly #host: SkipperHost;
  // waiting は、蓋を出して、飛び先が映るのを待っている間の状態。
  #waiting: { target: number; count: number; cancelTimer: () => void } | null = null;

  constructor(host: SkipperHost) {
    this.#host = host;
  }

  // waiting は、飛び先が映るのを待っているかどうか。
  get waiting(): boolean {
    return this.#waiting !== null;
  }

  // frame は、映ったコマごとに呼ぶ。
  frame(frame: SkipperFrame): void {
    const waiting = this.#waiting;
    if (waiting !== null) {
      if (!frame.presented) {
        return;
      }
      // seek の途中のコマと、飛び先より手前のコマが来たら、数え直す。
      const landed =
        !frame.seeking &&
        frame.position !== null &&
        frame.position >= waiting.target - landingTolerance;
      waiting.count = landed ? waiting.count + 1 : 0;
      if (waiting.count >= uncoverFrames) {
        this.#release();
      }

      return;
    }
    const host = this.#host;
    if (frame.paused || frame.position === null || !host.enabled() || host.scrubbing()) {
      return;
    }
    const sections = host.sections();
    const section = cmSectionAt(sections, frame.position, cmLookahead);
    if (!shouldAutoSkip(section, true, true, host.lastSeek())) {
      return;
    }
    // CM の中で写すと、蓋そのものが CM のコマになるので、いまのコマが本編のときだけ写す。
    const inCm = cmSectionAt(sections, frame.position) !== null;
    this.#hold(inCm ? 'black' : 'still', () => host.jump(section!.to));
    host.notify();
  }

  // holdUntil は、飛ばずに、黒い蓋で覆ったまま、target 以上のコマが映るのを待つ。開き始めに、最初から CM の終わりを
  // 求めたときに使う。配信は手前の I フレームから流れるので、CM の末尾が映りうる。
  holdUntil(target: number): void {
    this.#hold('black', () => target);
  }

  dispose(): void {
    this.#waiting?.cancelTimer();
    this.#waiting = null;
  }

  #hold(kind: CoverKind, move: () => number): void {
    this.#waiting?.cancelTimer();
    // 蓋を先に出してから移る。移ってから出すと、その間に CM のコマが出る。
    this.#host.cover(kind);
    const waiting = { target: Number.POSITIVE_INFINITY, count: 0, cancelTimer: () => {} };
    this.#waiting = waiting;
    waiting.target = move();
    waiting.cancelTimer = this.#host.setTimer(() => {
      if (this.#waiting === waiting) {
        this.#release();
      }
    }, coverTimeoutMs);
  }

  #release(): void {
    this.#waiting?.cancelTimer();
    this.#waiting = null;
    this.#host.uncover();
  }
}
