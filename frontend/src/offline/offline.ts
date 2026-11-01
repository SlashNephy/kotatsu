// 録画の端末への保存 (#480、設計書 7.8) の、保存先によらない決まりごと。
// 保存とオリジンのプライベートファイルシステム (OPFS) の読み書きは store.ts に置き、ここには判断だけを置く。

import type { Gap } from '../yunomi/position';

// OfflineMeta は、端末に保存した録画の情報。サーバーにつながらなくても、一覧と再生の画面を出せるだけを持つ。
// 映像のファイルを書き終えてから最後に書くので、これがあれば、映像は揃っている。
export type OfflineMeta = {
  version: 1;
  id: string;
  name: string;
  serviceName: string;
  startAt: string;
  endAt: string;
  // recordedFrom と gaps は、再生位置を放送時刻に直すのに使う (設計書 7.2)。
  recordedFrom: string;
  recordedUntil: string;
  gaps: Gap[];
  // size は、保存した映像のバイト数。
  size: number;
  // jikkyo は、実況の過去ログも保存したかどうか。
  jikkyo: boolean;
  // timeZone は、保存したときのサーバーの表示のタイムゾーン。オフラインでは読み込めないので、これで日時を書く。
  timeZone: string;
  savedAt: string;
};

// metaVersion は、OfflineMeta の形の版。形を変えたら上げ、古い版は読めないものとして扱う。
const metaVersion = 1;

const isString = (value: unknown): value is string => typeof value === 'string';

// parseMeta は、保存した録画の情報を読む。壊れているか、形の違うものなら null を返す。
// 書きかけで止まった保存や、古い版の情報を、保存した録画として並べないためである。
export function parseMeta(text: string): OfflineMeta | null {
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    return null;
  }
  if (typeof value !== 'object' || value === null) {
    return null;
  }
  const meta = value as Record<string, unknown>;
  if (
    meta.version !== metaVersion ||
    !isString(meta.id) ||
    !isString(meta.name) ||
    !isString(meta.serviceName) ||
    !isString(meta.startAt) ||
    !isString(meta.endAt) ||
    !isString(meta.recordedFrom) ||
    !isString(meta.recordedUntil) ||
    !isString(meta.timeZone) ||
    !isString(meta.savedAt) ||
    typeof meta.size !== 'number' ||
    typeof meta.jikkyo !== 'boolean' ||
    !Array.isArray(meta.gaps) ||
    !meta.gaps.every(
      (gap: unknown) =>
        typeof gap === 'object' &&
        gap !== null &&
        isString((gap as Gap).from) &&
        isString((gap as Gap).to),
    )
  ) {
    return null;
  }

  return meta as OfflineMeta;
}

// expectedSize は、保存する映像の大きさ (バイト) を見積もる。
// 配信が Content-Length を返せばそれを使い、なければ録画のセグメントの大きさの和を使う。
// セグメントの和は、引き継ぎで重なった区間を切り詰める前の大きさなので、実際より大きめになる (設計書 7.6)。
export function expectedSize(
  contentLength: string | null,
  files: readonly { kind: string; size: number | string }[],
): number {
  const length = contentLength === null ? Number.NaN : Number(contentLength);
  if (Number.isFinite(length) && length > 0) {
    return length;
  }

  return files
    .filter((file) => file.kind === 'SEGMENT')
    .reduce((sum, file) => sum + Number(file.size), 0);
}

// hasRoom は、端末の空き (navigator.storage.estimate の結果) に、required バイトを書けるかどうかを返す。
// ブラウザが上限を知らせないときは、確かめようがないので、書いてみて、溢れたらそこで止める。
export function hasRoom(estimate: StorageEstimate, required: number): boolean {
  if (estimate.quota === undefined) {
    return true;
  }

  return estimate.quota - (estimate.usage ?? 0) >= required;
}

// OfflineSupport は、端末への保存を使えるかどうか。使えなければ、その理由。
export type OfflineSupport = 'supported' | 'noStorage' | 'noConverter';

// offlineSupport は、この端末で、録画を保存して再生できるかを返す。
// 保存した TS は、ブラウザの中で変換して再生する (設計書 7.2) ので、変換の使えない端末 (iPhone など) と、
// ブラウザの中で変換できない BS4K と 8K の録画は、保存しても見られない。
export function offlineSupport(
  scope: typeof globalThis,
  convert: boolean,
  videoFormat: string,
): OfflineSupport {
  const storage = (scope.navigator as Navigator | undefined)?.storage as
    | Partial<StorageManager>
    | undefined;
  const handle = (scope as { FileSystemFileHandle?: { prototype: object } }).FileSystemFileHandle;
  if (
    typeof storage?.getDirectory !== 'function' ||
    typeof storage.estimate !== 'function' ||
    handle === undefined ||
    !('createWritable' in handle.prototype)
  ) {
    return 'noStorage';
  }
  if (!convert || videoFormat !== 'HD') {
    return 'noConverter';
  }

  return 'supported';
}

// progressRatio は、保存の進み具合 (0〜1) を返す。全体の大きさが分からなければ null。
export function progressRatio(received: number, total: number | null): number | null {
  if (total === null || total <= 0) {
    return null;
  }

  return Math.min(received / total, 1);
}
