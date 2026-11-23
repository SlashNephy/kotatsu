// PTS (90kHz、33 ビット) は約 26.5 時間ごとに一周する (#1516)。ブラウザ内でデコードする経路では、映像、音、字幕の時刻を
// 同じ物差しで比べるので、一周しても増え続ける数に伸ばしてから使う。

// ptsTicksPerSecond は、PTS の 1 秒。
export const ptsTicksPerSecond = 90_000;

// ptsWrap は、PTS が一周する数 (2^33)。
export const ptsWrap = 2 ** 33;

// PtsTimeline は、33 ビットの PTS を、一周しても増え続ける数に伸ばす。
// 前に伸ばした値に最も近い値を選ぶので、半周 (約 13 時間) より大きく飛ばなければ、前にも後ろにも正しく伸ばせる。
// 映像と音と字幕で、同じ PtsTimeline を使う。別々に伸ばすと、一周の前後で食い違うため。
export class PtsTimeline {
  private last: number | null = null;

  // extend は、33 ビットの PTS を、伸ばした値にする。
  extend(pts: number): number {
    const raw = ((pts % ptsWrap) + ptsWrap) % ptsWrap;
    if (this.last === null) {
      this.last = raw;

      return raw;
    }
    const base = this.last - (((this.last % ptsWrap) + ptsWrap) % ptsWrap);
    let extended = base + raw;
    if (extended - this.last > ptsWrap / 2) {
      extended -= ptsWrap;
    } else if (this.last - extended > ptsWrap / 2) {
      extended += ptsWrap;
    }
    this.last = extended;

    return extended;
  }

  // reset は、覚えた位置を忘れる。選局したときに呼ぶ (局ごとに PTS はばらばらなので)。
  reset(): void {
    this.last = null;
  }
}
