// 映像の左右の端を縦にスワイプして、明るさと音量を変える (#1598、設計書 7.5)。
// 指の動きを、縦か横かと、何段変えるかに読み替える。DOM には触らないので、テストで動きを直に渡せる。

// SwipeBand は、スワイプを受け付ける帯。左の帯は明るさ、右の帯は音量を変える。
export type SwipeBand = 'brightness' | 'volume';

// swipeBandWidth は、左右それぞれの帯の幅 (プレイヤーの幅に対する割合)。真ん中は、タップやダブルタップに残す。
export const swipeBandWidth = 0.3;
// swipeEdgeMargin は、画面の端からのジェスチャー (戻る、通知の引き出し) と取り合わないよう、帯から除く端の幅 (px)。
export const swipeEdgeMargin = 24;
// swipeThreshold は、縦か横かを決めるまでに、指が動く距離 (px)。これより短い動きは、タップとして残す。
export const swipeThreshold = 16;
// swipeStep は、1 段変えるのに、指が縦に動く距離 (px)。
export const swipeStep = 24;
// swipeVolumeStep は、音量の帯のスワイプ 1 段で変える音量 (0 から 2 の割合)。
export const swipeVolumeStep = 0.05;

// swipeBandAt は、横の位置 x (プレイヤーの左端から、幅 width) で始めたスワイプの帯を返す。帯の外や、
// 音量を変えられない端末 (iOS) の音量の帯なら null を返す。
export function swipeBandAt(x: number, width: number, volumeAvailable: boolean): SwipeBand | null {
  if (width <= 0 || x < swipeEdgeMargin || x > width - swipeEdgeMargin) {
    return null;
  }
  if (x < width * swipeBandWidth) {
    return 'brightness';
  }
  if (x > width * (1 - swipeBandWidth)) {
    return volumeAvailable ? 'volume' : null;
  }

  return null;
}

// SwipeTracker は、1 本の指のスワイプを追う。縦か横かは、指が swipeThreshold より遠くへ動いたときに決め、
// 指を離すまで変えない。横と決めたスワイプは、何も変えない。
export class SwipeTracker {
  direction: 'pending' | 'vertical' | 'horizontal' = 'pending';
  private applied = 0;

  constructor(
    readonly band: SwipeBand,
    private readonly startX: number,
    private readonly startY: number,
  ) {}

  // move は、指が (x, y) へ動いたときに、前に呼んだときから変える段の数を返す。上へ動かすと正、下へ動かすと負。
  move(x: number, y: number): number {
    if (this.direction === 'pending') {
      const dx = x - this.startX;
      const dy = y - this.startY;
      if (Math.hypot(dx, dy) < swipeThreshold) {
        return 0;
      }
      this.direction = Math.abs(dy) > Math.abs(dx) ? 'vertical' : 'horizontal';
    }
    if (this.direction !== 'vertical') {
      return 0;
    }
    const total = Math.trunc((this.startY - y) / swipeStep);
    const delta = total - this.applied;
    this.applied = total;

    return delta;
  }
}
