// L 字放送の切り抜き (#496、設計書 7.5)。災害や選挙の L 字放送で、番組の映像が画面の一部に縮んだときに、そこを拡大して見る。
// 映像と字幕をまとめて拡大し、枠の外に出た部分を隠す。字幕は映像の座標に載るので、一緒に動かさないとずれる。
// 4:3 の映像の左右の黒い縁を消す、横に伸ばす、といった表示の比率のプリセットも、同じ仕組みで出す (#824)。

// CropOrigin は、拡大の起点。L 字放送では、番組の映像は左上か右上に寄ることが多い。
export type CropOrigin = 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right' | 'center';

// Crop は、切り抜き。scale が 1 で stretchX がなければ切り抜かない。
// stretchX は、横だけに掛ける倍率 (#824)。4:3 の映像を横に伸ばして 16:9 の枠に合わせるのに使う。
export type Crop = { scale: number; origin: CropOrigin; stretchX?: number };

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

// isCropped は、切り抜くかどうかを返す。
export function isCropped(crop: Crop): boolean {
  return crop.scale !== 1 || (crop.stretchX ?? 1) !== 1;
}

// scales は、横と縦の倍率を返す。
function scales(crop: Crop): { x: number; y: number } {
  return { x: crop.scale * (crop.stretchX ?? 1), y: crop.scale };
}

// cropStyle は、映像と字幕を包む要素に掛ける CSS の変形を返す。切り抜かなければ空。
export function cropStyle(crop: Crop): { transform?: string; transformOrigin?: string } {
  if (!isCropped(crop)) {
    return {};
  }
  const { x, y } = anchor(crop.origin);
  const scale = scales(crop);

  return {
    transform: scale.x === scale.y ? `scale(${scale.x})` : `scale(${scale.x}, ${scale.y})`,
    transformOrigin: `${x * 100}% ${y * 100}%`,
  };
}

// CropMode は、表示の比率のメニューの選択肢 (#824)。lShape は、拡大率と起点を選ぶ L 字の切り抜き。
export type CropMode = 'none' | 'lShape' | 'pillarbox' | 'stretch';

export const cropModes: readonly CropMode[] = ['none', 'lShape', 'pillarbox', 'stretch'];

// fourThree は、16:9 の枠に入った 4:3 の映像を、枠に合わせる倍率。
const fourThree = 4 / 3;

// cropForMode は、プリセットの切り抜きを返す。lShape は、L 字の切り抜きの既定 (1.25 倍、左上)。
// pillarbox は、4:3 の映像の左右の黒い縁を消すよう拡大する (上下は切れる)。stretch は、4:3 の映像を横に伸ばして枠に合わせる。
export function cropForMode(mode: CropMode): Crop {
  switch (mode) {
    case 'none':
      return noCrop;
    case 'lShape':
      return { scale: 1.25, origin: 'top-left' };
    case 'pillarbox':
      return { scale: fourThree, origin: 'center' };
    case 'stretch':
      return { scale: 1, origin: 'center', stretchX: fourThree };
  }
}

// cropMode は、切り抜きが、メニューのどの選択肢に当たるかを返す。
export function cropMode(crop: Crop): CropMode {
  if (!isCropped(crop)) {
    return 'none';
  }
  if ((crop.stretchX ?? 1) !== 1) {
    return 'stretch';
  }

  return crop.scale === fourThree && crop.origin === 'center' ? 'pillarbox' : 'lShape';
}

// cropSource は、幅 width、高さ height の画像から、切り抜きで見えている範囲を返す。キャプチャで、同じ範囲を切り出すのに使う。
export function cropSource(
  crop: Crop,
  width: number,
  height: number,
): { x: number; y: number; width: number; height: number } {
  const scale = scales(crop);
  const shownWidth = width / scale.x;
  const shownHeight = height / scale.y;
  const { x, y } = anchor(crop.origin);

  return {
    x: (width - shownWidth) * x,
    y: (height - shownHeight) * y,
    width: shownWidth,
    height: shownHeight,
  };
}
