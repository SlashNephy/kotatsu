// 置き換えられない字幕の外字 (DRCS) を、なめらかな形にする (#1518、設計書 7.4)。
// 放送の外字は 16〜36 画素四方ほどの画像で、字の枠いっぱいに引き伸ばすと、1 点が 4x4 ほどの階段になる。
// ここでは、水増しを畳み、Scale2x (EPX) で拡大して角を丸め、二値に戻して、輪郭の path にする。
// 描画器には依存しない。描画器は、この path を、字の枠に縮めて塗る。

// DRCSImage は、外字の画像を、1 画素 1 バイトの濃さ (0〜255) に開いたもの。左上から行の順に並べる。
export type DRCSImage = {
  width: number;
  height: number;
  pixels: Uint8Array;
};

// DRCSShape は、なめらかにした外字の輪郭。width と height は、path の座標の広さ。
export type DRCSShape = {
  width: number;
  height: number;
  path: string;
};

// drcsTargetSize は、倍率 k を決めるときに目指す、一辺の画素数。
// 全画面 (1920x1080) で描いたときの字の枠 (36 の字を 2 倍した 72 画素) に合わせる。
// これより小さいと、引き伸ばしで階段が残る。大きくしても、字の枠より細かい形は見えない。
export const drcsTargetSize = 72;

// drcsMaxSize は、倍率 k を掛けた一辺の上限。
// path を描く受け側には画像の大きさの上限が無いので、手間 (Scale2x の画素の数と path の長さ) で決める。
// 目標の一辺が 72 なので、k を掛けた一辺は、外字が 72 より小さければ 144 未満に収まる。上限は、それより大きい外字の守りにする。
export const drcsMaxSize = 256;

// drcsSupersampling は、k に重ねる超標本化の倍率 m (2 の冪)。path は、字の枠の 1 画素を m 個に分けた細かさになる。
// canvas の path の塗りには補間がかかるので、2 で縁はなめらかになる。
// Scale2x を重ねても形はほとんど変わらず、path の点が倍に増えるだけなので、これより大きくしない。
export const drcsSupersampling = 2;

// drcsThreshold は、二値に戻すときに、塗りとする濃さの下限。濃さ (0〜255) の中間に置く。
// 2 階調の外字では、どの値でも同じになる。階調の多い外字では、高いと線が細り、低いと線が太る。
export const drcsThreshold = 128;

// openDRCS は、aribb24.js の外字の画像を、濃さに開く (手順 1)。読めない画像は null にする。
// depth は、aribb24.js が渡す 1 画素のビット数 (ARIB の depth の値ではない)。階調の数は 2 の depth 乗と見る。
export function openDRCS(glyph: {
  width: number;
  height: number;
  depth: number;
  binary: ArrayBuffer;
}): DRCSImage | null {
  const { width, height, depth } = glyph;
  if (
    !Number.isInteger(width) ||
    !Number.isInteger(height) ||
    width <= 0 ||
    height <= 0 ||
    ![1, 2, 4, 8].includes(depth)
  ) {
    return null;
  }
  const max = 2 ** depth - 1;
  const bytes = new Uint8Array(glyph.binary);
  const pixels = new Uint8Array(width * height);
  for (let i = 0; i < pixels.length; i++) {
    let value = 0;
    for (let bit = 0; bit < depth; bit++) {
      const index = i * depth + bit;
      value = (value << 1) | (((bytes[index >> 3] ?? 0) >> (7 - (index & 7))) & 1);
    }
    pixels[i] = Math.round((value * 255) / max);
  }

  return { width, height, pixels };
}

// foldDRCS は、縦か横に整数倍して送られた外字を、元の大きさに畳む (手順 2)。畳めなくなるまで繰り返す。
// 畳まないと、2x2 の塊は斜めに接する点を持たないので、Scale2x がほとんど丸めない。
export function foldDRCS(image: DRCSImage): DRCSImage {
  let current = image;
  for (;;) {
    const columns = canFoldColumns(current);
    const rows = canFoldRows(current);
    if (!columns && !rows) {
      return current;
    }
    // 列と行は、同じ回で別々に判定してから畳む。
    const width = columns ? current.width / 2 : current.width;
    const height = rows ? current.height / 2 : current.height;
    const pixels = new Uint8Array(width * height);
    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        pixels[y * width + x] =
          current.pixels[(rows ? y * 2 : y) * current.width + (columns ? x * 2 : x)]!;
      }
    }
    current = { width, height, pixels };
  }
}

function canFoldColumns({ width, height, pixels }: DRCSImage): boolean {
  if (width % 2 !== 0) {
    return false;
  }
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x += 2) {
      if (pixels[y * width + x] !== pixels[y * width + x + 1]) {
        return false;
      }
    }
  }

  return true;
}

function canFoldRows({ width, height, pixels }: DRCSImage): boolean {
  if (height % 2 !== 0) {
    return false;
  }
  for (let y = 0; y < height; y += 2) {
    for (let x = 0; x < width; x++) {
      if (pixels[y * width + x] !== pixels[(y + 1) * width + x]) {
        return false;
      }
    }
  }

  return true;
}

// drcsScale は、畳んだ後の長い辺 size に掛ける倍率 k を返す (手順 3)。
// 1 から始め、size × k が target に届くまで 2 倍にする。ただし、2 倍にすると limit を越えるときは、そこで止める。
export function drcsScale(size: number, target = drcsTargetSize, limit = drcsMaxSize): number {
  let k = 1;
  while (size * k < target && size * k * 2 <= limit) {
    k *= 2;
  }

  return k;
}

// reflect は、画像の外の位置を、縁で折り返した内側の位置にする。一辺が 1 のときは 0 に留める。
// こうすると、縁に接した塗りの角が欠けず、縁へ抜ける斜めの線にも余計な点が付かない。
function reflect(index: number, size: number): number {
  if (size === 1) {
    return 0;
  }
  if (index < 0) {
    return 1;
  }
  if (index >= size) {
    return size - 2;
  }

  return index;
}

// scale2x は、Scale2x (EPX) で、縦横を 2 倍にする (手順 4)。縦横の線はまっすぐのまま、斜めの階段の間を埋める。
export function scale2x({ width, height, pixels }: DRCSImage): DRCSImage {
  const at = (x: number, y: number) => pixels[reflect(y, height) * width + reflect(x, width)]!;
  const outWidth = width * 2;
  const out = new Uint8Array(outWidth * height * 2);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const p = at(x, y);
      const up = at(x, y - 1);
      const right = at(x + 1, y);
      const left = at(x - 1, y);
      const down = at(x, y + 1);
      const top = y * 2 * outWidth + x * 2;
      out[top] = left === up && left !== down && up !== right ? up : p;
      out[top + 1] = up === right && up !== left && right !== down ? right : p;
      out[top + outWidth] = down === left && down !== right && left !== up ? left : p;
      out[top + outWidth + 1] = right === down && right !== up && down !== left ? down : p;
    }
  }

  return { width: outWidth, height: height * 2, pixels: out };
}

// outlinePath は、塗った画素 (filled が真) の輪郭を、SVG の path にする。
// 正方形を並べると、点の数が画素の数に比例して増えるので、輪郭だけをなぞる。
// 塗りを右手に見て回る向きにそろえるので、穴は逆向きになり、非ゼロの規則で塗れば穴が抜ける。
export function outlinePath(width: number, height: number, filled: (i: number) => boolean): string {
  const stride = width + 1;
  // 頂点ごとの出ていく辺の先。斜めに接する画素の角では 2 本になる。
  const next: number[][] = [];
  const add = (from: number, to: number) => {
    (next[from] ??= []).push(to);
  };
  const isFilled = (x: number, y: number) =>
    x >= 0 && y >= 0 && x < width && y < height && filled(y * width + x);
  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      if (!isFilled(x, y)) {
        continue;
      }
      const v = y * stride + x;
      if (!isFilled(x, y - 1)) {
        add(v, v + 1);
      }
      if (!isFilled(x + 1, y)) {
        add(v + 1, v + 1 + stride);
      }
      if (!isFilled(x, y + 1)) {
        add(v + 1 + stride, v + stride);
      }
      if (!isFilled(x - 1, y)) {
        add(v + stride, v);
      }
    }
  }
  // どの頂点も、入る辺と出ていく辺の数が等しいので、辿れば始めの頂点に戻る。
  // 2 本ある頂点でどちらに進んでも、辺の集まりは変わらないので、塗りは変わらない。
  const commands: string[] = [];
  for (let start = 0; start < next.length; start++) {
    while ((next[start]?.length ?? 0) > 0) {
      const loop: number[] = [];
      let v = start;
      do {
        loop.push(v);
        v = next[v]!.pop()!;
      } while (v !== start);
      commands.push(loopPath(loop, stride));
    }
  }

  return commands.join(' ');
}

// loopPath は、1 周の頂点の並びを、曲がり角だけの path にする。辺は縦か横なので、H と V で書く。
function loopPath(loop: readonly number[], stride: number): string {
  const corners: [number, number][] = [];
  for (let i = 0; i < loop.length; i++) {
    const prev = loop[(i + loop.length - 1) % loop.length]!;
    const v = loop[i]!;
    const following = loop[(i + 1) % loop.length]!;
    // 前の辺と次の辺が同じ向きなら、曲がり角ではない。
    if (v - prev !== following - v) {
      corners.push([v % stride, Math.floor(v / stride)]);
    }
  }
  const [first, ...rest] = corners;
  if (first === undefined) {
    return '';
  }
  let path = `M${first[0]} ${first[1]}`;
  let y = first[1];
  // 曲がり角は、縦と横の移動が交互に来るので、y が変わらなければ横の移動である。
  for (const [cx, cy] of rest) {
    path += cy === y ? `H${cx}` : `V${cy}`;
    y = cy;
  }

  return `${path}Z`;
}

// smoothDRCS は、外字の画像を、なめらかにした輪郭にする (手順 1〜4 と、二値に戻して path にする)。
// 読めない画像は null にする。描く大きさは、受け側が、元の画像の大きさに合わせて決める。
export function smoothDRCS(glyph: {
  width: number;
  height: number;
  depth: number;
  binary: ArrayBuffer;
}): DRCSShape | null {
  const opened = openDRCS(glyph);
  if (opened === null) {
    return null;
  }
  let image = foldDRCS(opened);
  const k = drcsScale(Math.max(image.width, image.height));
  for (let scale = 1; scale < k * drcsSupersampling; scale *= 2) {
    image = scale2x(image);
  }
  const { width, height, pixels } = image;

  return { width, height, path: outlinePath(width, height, (i) => pixels[i]! >= drcsThreshold) };
}
