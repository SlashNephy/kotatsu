// 番組の検索と録画の検索で、番組名のうちキーワードに当たった所にマークを付ける (#966)。
// tenban はキーワードと番組名を正規化してから照らすので (設計書 3.5)、ここでも同じく、NFKC で正規化し、波ダッシュを ~ に、
// ダッシュの類を - にそろえて (#2041) 照らし、当たった所を、画面に出す番組名の位置に戻す。
// マークは見た目の手がかりで、一致するかどうかは tenban が決める。正規表現は、tenban (Go) と同じ書き方の範囲で、JavaScript で照らす。

// HighlightKeyword は、番組名で探したキーワード。
export type HighlightKeyword = {
  readonly text: string;
  readonly regexp: boolean;
  readonly caseSensitive: boolean;
};

// HighlightRange は、マークを付ける範囲 (UTF-16 の位置。start を含み、end を含まない)。
export type HighlightRange = { start: number; end: number };

// foldDash は、tenban の domain.Normalize と同じく、波ダッシュを ~ に、ダッシュの類を - にする。
function foldDash(text: string): string {
  return text.replace(/〜/g, '~').replace(/[‐‒-―−]/g, '-');
}

// normalizeKeyword は、キーワードを、照らす文字列と同じく正規化する。
function normalizeKeyword(text: string, caseSensitive: boolean): string {
  const normalized = foldDash(text.normalize('NFKC'));

  return caseSensitive ? normalized : normalized.toLowerCase();
}

// Normalized は、正規化した文字列と、その 1 文字 (UTF-16) ごとの、元の文字列での範囲。
type Normalized = { text: string; starts: number[]; ends: number[] };

// normalizeWithPositions は、text を 1 文字 (コードポイント) ずつ正規化し、正規化した文字ごとに、元の位置を保持する。
// 1 文字ずつ正規化するので、半角のカタカナと濁点のような、つないで 1 文字になる組は、つながない。
function normalizeWithPositions(text: string, caseSensitive: boolean): Normalized {
  const result: Normalized = { text: '', starts: [], ends: [] };
  let index = 0;
  for (const char of text) {
    const normalized = normalizeKeyword(char, caseSensitive);
    for (let i = 0; i < normalized.length; i++) {
      result.starts.push(index);
      result.ends.push(index + char.length);
    }
    result.text += normalized;
    index += char.length;
  }

  return result;
}

// findMatches は、正規化した文字列 haystack の中で、keyword に当たる範囲 (正規化した文字列での位置) を返す。
function findMatches(haystack: string, keyword: HighlightKeyword): HighlightRange[] {
  const ranges: HighlightRange[] = [];
  if (keyword.regexp) {
    let pattern: RegExp;
    try {
      pattern = new RegExp(
        foldDash(keyword.text.normalize('NFKC')),
        keyword.caseSensitive ? 'gu' : 'giu',
      );
    } catch {
      // JavaScript で読めない正規表現 (Go だけの書き方) には、マークを付けない。
      return ranges;
    }
    for (const match of haystack.matchAll(pattern)) {
      if (match[0] !== '') {
        ranges.push({ start: match.index, end: match.index + match[0].length });
      }
    }

    return ranges;
  }
  const needle = normalizeKeyword(keyword.text, keyword.caseSensitive);
  if (needle === '') {
    return ranges;
  }
  for (
    let at = haystack.indexOf(needle);
    at !== -1;
    at = haystack.indexOf(needle, at + needle.length)
  ) {
    ranges.push({ start: at, end: at + needle.length });
  }

  return ranges;
}

// highlightRanges は、text のうち、keywords のどれかに当たる範囲を、元の text の位置で、前から順に重ならないように返す。
export function highlightRanges(
  text: string,
  keywords: readonly HighlightKeyword[],
): HighlightRange[] {
  const ranges: HighlightRange[] = [];
  for (const keyword of keywords) {
    if (keyword.text.trim() === '') {
      continue;
    }
    const normalized = normalizeWithPositions(text, keyword.caseSensitive);
    for (const match of findMatches(normalized.text, keyword)) {
      ranges.push({
        start: normalized.starts[match.start] ?? 0,
        end: normalized.ends[match.end - 1] ?? 0,
      });
    }
  }
  ranges.sort((a, b) => a.start - b.start || a.end - b.end);

  // 重なる範囲と、接する範囲は 1 つにする。
  const merged: HighlightRange[] = [];
  for (const range of ranges) {
    const last = merged.at(-1);
    if (last !== undefined && range.start <= last.end) {
      last.end = Math.max(last.end, range.end);
    } else {
      merged.push({ ...range });
    }
  }

  return merged;
}
