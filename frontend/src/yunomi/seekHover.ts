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

// SeekBand は、シークバーに示す区間 (#479)。from と to は、録画の始まりからの秒。同じなら、その位置の印にする。
// label は、カーソルを乗せたときに、吹き出しに添える文。kind が cm なら、CM とみなした区間として、ほかの区間と違う色で示す。
// kind が part なら、実況のコメントから見つけた番組の区切りの印 (#1358) として、押すとその位置へ移る。
export type SeekBand = { from: number; to: number; label: string; kind?: 'cm' | 'part' };

// bandAt は、seconds の位置にある区間を返す。印 (長さのない区間) は、前後 tolerance 秒の内なら当たったとみなす。なければ null。
export function bandAt(
  bands: readonly SeekBand[],
  seconds: number,
  tolerance: number,
): SeekBand | null {
  return (
    bands.find((band) => {
      // 区間は、バーの上で幅があるので、その中だけで当たったとみなす。
      const margin = band.to > band.from ? 0 : tolerance;

      return seconds >= band.from - margin && seconds <= band.to + margin;
    }) ?? null
  );
}

// partAt は、seconds から tolerance 秒の内で、いちばん近い番組の区切りの印を返す。なければ null。CM の区間の中でも、印を優先する。
export function partAt(
  bands: readonly SeekBand[],
  seconds: number,
  tolerance: number,
): SeekBand | null {
  let nearest: SeekBand | null = null;
  for (const band of bands) {
    const distance = Math.abs(band.from - seconds);
    if (
      band.kind === 'part' &&
      distance <= tolerance &&
      (nearest === null || distance < Math.abs(nearest.from - seconds))
    ) {
      nearest = band;
    }
  }

  return nearest;
}
