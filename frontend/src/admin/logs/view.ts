// ログの画面の並べ方と絞り込み (#1521、設計書 11.3)。

import type { Level } from './report';

// pageSize は、画面に一度に描く行数と、「もっと見る」で増やす行数 (1 回に描く行数、設計書 付録 A)。
export const pageSize = 200;

export type ViewLine = {
  // source は、出どころの番号 (sources の添え字)。
  source: number;
  time: number;
  level: Level;
  inferred: boolean;
  text: string;
};

export type ViewFilter = {
  // hidden は、表示を切った出どころの番号。
  hidden: ReadonlySet<number>;
  // problemsOnly は、警告とエラーだけを出すかどうか。
  problemsOnly: boolean;
  // search は、大文字と小文字を区別しない、部分一致の検索の語。空なら絞らない。
  search: string;
};

// mergeLines は、出どころごとの古い順の行を、時刻の新しい順に混ぜて並べる。同じ時刻なら、出どころの中の順を保つ。
export function mergeLines(
  sources: readonly { lines: readonly Omit<ViewLine, 'source'>[] }[],
): ViewLine[] {
  const merged = sources.flatMap((source, index) =>
    source.lines.map((line, order) => ({ line: { ...line, source: index }, order })),
  );
  merged.sort(
    (a, b) => b.line.time - a.line.time || a.line.source - b.line.source || b.order - a.order,
  );

  return merged.map(({ line }) => line);
}

// filterLines は、絞り込みに合う行を返す。
export function filterLines(
  lines: readonly ViewLine[],
  { hidden, problemsOnly, search }: ViewFilter,
): ViewLine[] {
  const needle = search.trim().toLowerCase();

  return lines.filter(
    (line) =>
      !hidden.has(line.source) &&
      (!problemsOnly || line.level !== 'INFO') &&
      (needle === '' || line.text.toLowerCase().includes(needle)),
  );
}
