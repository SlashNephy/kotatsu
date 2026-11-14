// 映像を長押ししたまま横に滑らせて、再生位置を動かす (#1601、設計書 7.5)。シークバーは細く、指で目的の位置に
// 合わせにくいので、映像の広い面を使って細かく合わせる。指の動きを、行き先の秒に読み替える。DOM には触らない。

// slideSeekHoldMs は、長押しとみなすまで、指を動かさずに押し続ける時間。
export const slideSeekHoldMs = 500;
// slideSeekMoveLimit は、長押しになる前に動かしたら、長押しにしない距離 (px)。スワイプやスクロールに残す。
export const slideSeekMoveLimit = 10;
// slideSeekThreshold は、長押しの後、滑らせ始めたとみなす横の距離 (px)。これより短ければ、離したときにメニューを開く。
export const slideSeekThreshold = 16;
// slideSeekSecondsPerWidth は、プレイヤーの幅いっぱいに滑らせたときに動く秒。
export const slideSeekSecondsPerWidth = 120;

// SlideSeekTracker は、長押しの後に横に滑らせる指を追い、行き先の秒を返す。
export class SlideSeekTracker {
  sliding = false;
  private originX: number;
  private originSeconds: number;

  constructor(
    startX: number,
    startSeconds: number,
    private readonly duration: number,
    private readonly width: number,
  ) {
    this.originX = startX;
    this.originSeconds = startSeconds;
  }

  // move は、指が x へ動いたときの行き先の秒を返す。滑らせ始める前なら null を返す。
  // 端 (0 か duration) で止まったら、溜まった移動量を切り詰め、戻す向きに動かしたらすぐに効くようにする。
  move(x: number): number | null {
    if (!this.sliding) {
      if (Math.abs(x - this.originX) < slideSeekThreshold) {
        return null;
      }
      this.sliding = true;
      this.originX = x;
    }
    const raw =
      this.originSeconds +
      ((x - this.originX) * slideSeekSecondsPerWidth) / Math.max(this.width, 1);
    const seconds = Math.min(this.duration, Math.max(0, raw));
    if (seconds !== raw) {
      this.originX = x;
      this.originSeconds = seconds;
    }

    return seconds;
  }
}
