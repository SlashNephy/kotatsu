import type { JikkyoComment } from './comments';

// 実況のコメントから番組の区切りを見つける (#1358、設計書 7.4)。
// 実況では、番組の区切りで OP、A、B、ED、C と書き込む習慣がある。同じ記号が数秒の間に固まった位置を、区切りとする。

export type PartSymbol = 'OP' | 'A' | 'B' | 'ED' | 'C';

const partSymbols: ReadonlySet<string> = new Set<PartSymbol>(['OP', 'A', 'B', 'ED', 'C']);

// PartMarker は、区切り。time は、固まりの最初の書き込みの時刻 (録画の始まりからの秒)。
// noCmBefore は、その前に CM を挟まない区切りかどうか。
export type PartMarker = { part: PartSymbol; time: number; noCmBefore: boolean };

// maxGapSeconds は、同じ記号の書き込みを 1 つの固まりとみなす間隔の上限。minPosts は、区切りとみなす固まりの件数の下限。
const maxGapSeconds = 8;
const minPosts = 2;
// mergeSeconds は、同じ記号の固まりを 1 つの区切りにまとめる間隔。区切りは数分は離れるので、近い固まりは同じ場面への書き込みとみなす。
const mergeSeconds = 60;
// noCmWithinSeconds は、OP の開始からこの秒数の内の A は、OP の後に CM を挟んでいないとみなす間隔。
const noCmWithinSeconds = 120;
// maxSymbolLength は、記号とみなす本文の長さの上限。長い本文は、正規化せずに読み飛ばす。
const maxSymbolLength = 8;

// toSymbol は、本文を区切りの記号に直す。全角と半角は同じ記号にする。小文字は連投が多いので、記号にしない。
function toSymbol(content: string): PartSymbol | null {
  if (content.length > maxSymbolLength) {
    return null;
  }
  const normalized = content.normalize('NFKC').trim();

  return partSymbols.has(normalized) ? (normalized as PartSymbol) : null;
}

// findPartMarkers は、時刻の順に並んだコメントから、区切りを時刻の順に返す。
export function findPartMarkers(comments: readonly JikkyoComment[]): PartMarker[] {
  const clusters: { part: PartSymbol; time: number }[] = [];
  const open = new Map<PartSymbol, { first: number; last: number; posts: number }>();
  const close = (part: PartSymbol) => {
    const cluster = open.get(part);
    if (cluster !== undefined && cluster.posts >= minPosts) {
      clusters.push({ part, time: cluster.first });
    }
  };
  for (const comment of comments) {
    const part = toSymbol(comment.content);
    if (part === null) {
      continue;
    }
    const cluster = open.get(part);
    if (cluster !== undefined && comment.time - cluster.last <= maxGapSeconds) {
      cluster.last = comment.time;
      cluster.posts++;
      continue;
    }
    close(part);
    open.set(part, { first: comment.time, last: comment.time, posts: 1 });
  }
  for (const part of open.keys()) {
    close(part);
  }
  clusters.sort((a, b) => a.time - b.time);

  const markers: PartMarker[] = [];
  const lastTime = new Map<PartSymbol, number>();
  let lastOp: number | null = null;
  for (const { part, time } of clusters) {
    const previous = lastTime.get(part);
    lastTime.set(part, time);
    if (previous !== undefined && time - previous <= mergeSeconds) {
      continue;
    }
    markers.push({
      part,
      time,
      noCmBefore: part === 'A' && lastOp !== null && time - lastOp <= noCmWithinSeconds,
    });
    if (part === 'OP') {
      lastOp = time;
    }
  }

  return markers;
}
