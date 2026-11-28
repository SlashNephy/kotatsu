// シークバー用のプレビュー画像 (preview.jpg) から、1 フレームを切り出す。
// フレームの並びは、同じディレクトリの preview.json (tenban の thumbnail.Manifest) に書いてある。
// 並べ方は、tenban の thumbnail.tile() と揃える (1 行に columns 個。フレームが columns 個より少なければ、その数の幅)。

export type PreviewManifest = {
  // width と height は、1 フレームの大きさ。
  width: number;
  height: number;
  // columns は、1 行のフレームの数。フレームは、左上から右へ、行が埋まったら次の行へ並ぶ。
  columns: number;
  // times は、各フレームの、録画の始まりからの時間 (秒)。
  times: number[];
};

export type PreviewFrame = {
  // backgroundSize と backgroundPosition は、preview.jpg を背景にして 1 フレームだけを見せるための CSS の値。
  backgroundSize: string;
  backgroundPosition: string;
};

// manifestUrl は、preview.jpg の URL を、同じディレクトリの preview.json の URL にする。
export function manifestUrl(previewUrl: string): string {
  // 作り直したら変わるよう、URL に添えたバージョン (?v=) も引き継ぐ。
  return previewUrl.replace(/preview\.jpg(\?|$)/, 'preview.json$1');
}

// previewFrame は、index 番目のフレームを切り出す CSS の値を返す。そのフレームがなければ null。
export function previewFrame(manifest: PreviewManifest, index: number): PreviewFrame | null {
  const count = manifest.times.length;
  if (manifest.columns <= 0 || !Number.isInteger(index) || index < 0 || index >= count) {
    return null;
  }
  const columns = Math.min(manifest.columns, count);
  const rows = Math.ceil(count / manifest.columns);
  // 背景を、フレームの数の倍に広げる。位置の割合は、端のフレームが 0% と 100% になる。
  const percent = (n: number, size: number) => (size <= 1 ? 0 : (n / (size - 1)) * 100);

  return {
    backgroundSize: `${columns * 100}% ${rows * 100}%`,
    backgroundPosition: `${percent(index % manifest.columns, columns)}% ${percent(Math.floor(index / manifest.columns), rows)}%`,
  };
}

// frameBefore は、録画の始まりから seconds 秒以前で、いちばん近いフレームの番号を返す。それより前にフレームがなければ null。
// 後のフレームは選ばないので、その位置より先の場面を見せない (tenban の thumbnail.Manifest.FrameBefore と同じ)。times は小さい順に並んでいる。
export function frameBefore(times: readonly number[], seconds: number): number | null {
  let found: number | null = null;
  for (const [index, time] of times.entries()) {
    if (time > seconds) {
      break;
    }
    found = index;
  }

  return found;
}

// slideshowFrames は、count 個のフレームから、録画全体にわたって等間隔に、多くても limit 個を選んだ番号を返す。
export function slideshowFrames(count: number, limit: number): number[] {
  if (count <= 0 || limit <= 0) {
    return [];
  }
  const n = Math.min(count, limit);

  return Array.from({ length: n }, (_, i) => Math.floor((i * count) / n));
}

// FramePosition は、tenban が求めた、プレビュー画像の 1 フレームの位置 (GraphQL の PreviewFrame、ピクセル)。
export type FramePosition = {
  x: number;
  y: number;
  width: number;
  height: number;
  sheetWidth: number;
  sheetHeight: number;
};

// framePosition は、tenban が求めたフレームの位置を、1 フレームだけを見せる CSS の値にする。
export function framePosition(frame: FramePosition): PreviewFrame {
  // 位置の割合は、端のフレームが 0% と 100% になる。プレビュー画像がフレーム 1 つ分の幅なら 0%。
  const percent = (at: number, size: number, sheet: number) =>
    sheet <= size ? 0 : (at / (sheet - size)) * 100;

  return {
    backgroundSize: `${(frame.sheetWidth / frame.width) * 100}% ${(frame.sheetHeight / frame.height) * 100}%`,
    backgroundPosition: `${percent(frame.x, frame.width, frame.sheetWidth)}% ${percent(frame.y, frame.height, frame.sheetHeight)}%`,
  };
}
