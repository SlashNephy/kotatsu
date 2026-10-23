// L 字放送の切り抜き (#496、設計書 7.5)。災害や選挙の L 字放送で、番組の映像が画面の一部に縮んだときに、そこを拡大して見る。
// 映像と字幕をまとめて拡大し、枠の外に出た部分を隠す。字幕は映像の座標に載るので、一緒に動かさないとずれる。

// CropOrigin は、拡大の起点。L 字放送では、番組の映像は左上か右上に寄ることが多い。
export type CropOrigin = 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right' | 'center';

// Crop は、切り抜き。scale が 1 なら切り抜かない。
export type Crop = { scale: number; origin: CropOrigin };

export const noCrop: Crop = { scale: 1, origin: 'top-left' };

// cropScales は、選べる拡大率。L 字の帯は、画面の 1 割から 2 割ほどを占める。
export const cropScales = [1, 1.1, 1.2, 1.25, 1.33, 1.5] as const;

export const cropOrigins: readonly CropOrigin[] = [
  'top-left',
  'top-right',
  'bottom-left',
  'bottom-right',
  'center',
];

// anchor は、起点の、枠の中の位置 (0 から 1)。
function anchor(origin: CropOrigin): { x: number; y: number } {
  switch (origin) {
    case 'top-left':
      return { x: 0, y: 0 };
    case 'top-right':
      return { x: 1, y: 0 };
    case 'bottom-left':
      return { x: 0, y: 1 };
    case 'bottom-right':
      return { x: 1, y: 1 };
    case 'center':
      return { x: 0.5, y: 0.5 };
  }
}

// cropStyle は、映像と字幕を包む要素に掛ける CSS の変形を返す。切り抜かなければ空。
export function cropStyle(crop: Crop): { transform?: string; transformOrigin?: string } {
  if (crop.scale === 1) {
    return {};
  }
  const { x, y } = anchor(crop.origin);

  return { transform: `scale(${crop.scale})`, transformOrigin: `${x * 100}% ${y * 100}%` };
}

// cropSource は、幅 width、高さ height の画像から、切り抜きで見えている範囲を返す。キャプチャで、同じ範囲を切り出すのに使う。
export function cropSource(
  crop: Crop,
  width: number,
  height: number,
): { x: number; y: number; width: number; height: number } {
  const shownWidth = width / crop.scale;
  const shownHeight = height / crop.scale;
  const { x, y } = anchor(crop.origin);

  return {
    x: (width - shownWidth) * x,
    y: (height - shownHeight) * y,
    width: shownWidth,
    height: shownHeight,
  };
}
