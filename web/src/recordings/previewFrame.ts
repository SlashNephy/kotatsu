// シークバー用のプレビュー画像 (preview.jpg) から、1 コマを切り出す。
// コマの並びは、同じディレクトリの preview.json (tenban の thumbnail.Manifest) に書いてある。
// 並べ方は、tenban の thumbnail.tile() と揃える (1 行に columns 個。コマが columns 個より少なければ、その数の幅)。

export type PreviewManifest = {
  // width と height は、1 コマの大きさ。
  width: number;
  height: number;
  // columns は、1 行のコマの数。コマは、左上から右へ、行が埋まったら次の行へ並ぶ。
  columns: number;
  // times は、各コマの、録画の始まりからの時間 (秒)。
  times: number[];
};

export type PreviewFrame = {
  // backgroundSize と backgroundPosition は、preview.jpg を背景にして 1 コマだけを見せるための CSS の値。
  backgroundSize: string;
  backgroundPosition: string;
};

// manifestUrl は、preview.jpg の URL を、同じディレクトリの preview.json の URL にする。
export function manifestUrl(previewUrl: string): string {
  // 作り直したら変わるよう、URL に添えた版 (?v=) も引き継ぐ。
  return previewUrl.replace(/preview\.jpg(\?|$)/, 'preview.json$1');
}

// previewFrame は、index 番目のコマを切り出す CSS の値を返す。そのコマがなければ null。
export function previewFrame(manifest: PreviewManifest, index: number): PreviewFrame | null {
  const count = manifest.times.length;
  if (manifest.columns <= 0 || !Number.isInteger(index) || index < 0 || index >= count) {
    return null;
  }
  const columns = Math.min(manifest.columns, count);
  const rows = Math.ceil(count / manifest.columns);
  // 背景を、コマの数の倍に広げる。位置の割合は、端のコマが 0% と 100% になる。
  const percent = (n: number, size: number) => (size <= 1 ? 0 : (n / (size - 1)) * 100);

  return {
    backgroundSize: `${columns * 100}% ${rows * 100}%`,
    backgroundPosition: `${percent(index % manifest.columns, columns)}% ${percent(Math.floor(index / manifest.columns), rows)}%`,
  };
}

// slideshowFrames は、count 個のコマから、録画全体にわたって等間隔に、多くても limit 個を選んだ番号を返す。
export function slideshowFrames(count: number, limit: number): number[] {
  if (count <= 0 || limit <= 0) {
    return [];
  }
  const n = Math.min(count, limit);

  return Array.from({ length: n }, (_, i) => Math.floor((i * count) / n));
}

// FramePosition は、tenban が求めた、プレビュー画像の 1 コマの位置 (GraphQL の PreviewFrame、ピクセル)。
export type FramePosition = {
  x: number;
  y: number;
  width: number;
  height: number;
  sheetWidth: number;
  sheetHeight: number;
};

// framePosition は、tenban が求めたコマの位置を、1 コマだけを見せる CSS の値にする。
export function framePosition(frame: FramePosition): PreviewFrame {
  // 位置の割合は、端のコマが 0% と 100% になる。プレビュー画像がコマ 1 つ分の幅なら 0%。
  const percent = (at: number, size: number, sheet: number) =>
    sheet <= size ? 0 : (at / (sheet - size)) * 100;

  return {
    backgroundSize: `${(frame.sheetWidth / frame.width) * 100}% ${(frame.sheetHeight / frame.height) * 100}%`,
    backgroundPosition: `${percent(frame.x, frame.width, frame.sheetWidth)}% ${percent(frame.y, frame.height, frame.sheetHeight)}%`,
  };
}
