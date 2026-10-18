// シークバーにカーソルを乗せたときに出す、位置の吹き出しの計算 (#755)。

// hoverSeconds は、シークバーの左端 left から幅 width の中で、カーソルの x に当たる秒を返す。バーの外は端に寄せる。
export function hoverSeconds(x: number, left: number, width: number, max: number): number {
  if (width <= 0) {
    return 0;
  }

  return Math.min(Math.max((x - left) / width, 0), 1) * max;
}

// bubbleLeft は、幅 bubble の吹き出しの中心を、幅 width のシークバーの中の offset に置くときの、中心の位置を返す。
// 吹き出しが端からはみ出さないよう、内側に寄せる。シークバーが吹き出しより狭ければ、真ん中に置く。
export function bubbleLeft(offset: number, width: number, bubble: number): number {
  if (width <= bubble) {
    return width / 2;
  }

  return Math.min(Math.max(offset, bubble / 2), width - bubble / 2);
}
