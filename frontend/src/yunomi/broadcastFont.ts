import broadcastFontUrl from '../../vendor/caption-font/rounded-mplus-1m-wadalab-mix-base.woff2?url';
import extendedRange from '../../vendor/caption-font/rounded-mplus-1m-wadalab-mix-jisx0213.range.json';
import broadcastExtendedFontUrl from '../../vendor/caption-font/rounded-mplus-1m-wadalab-mix-jisx0213.woff2?url';

// 字幕とデータ放送の文字を描くフォント (Rounded M+ 1m WadaLab mix、Unlicense) は、ビルドのときに 2 つに分ける (#1520、設計書 7.4)。
// ARIB の追加記号を含む等幅の丸ゴシックで、受信機の字幅を前提に作られた字幕とデータ放送の文書を、OS によらず同じ幅で描ける。
// - broadcastFontUrl は、基本の字のファイル (WOFF2 で 1.6 MB ほど)。字幕かデータ放送を描くときに初めて読む。
//   データ放送の文書 (BML) は EUC-JP の字と追加記号だけで書かれるので、これだけで足りる。
// - broadcastExtendedFontUrl は、JIS X 0213 で足された漢字 (第 3・第 4 水準) のファイル (1 MB ほど)。
//   字幕にめったに出ないので、その字が出たときだけ読む。
// どちらも Vite が中身のハッシュを名前に入れて出す資産なので、URL がそのまま版になる。
export { broadcastFontUrl, broadcastExtendedFontUrl };

// unicodeRangePattern は、CSS の unicode-range の書き方 (U+16進 か U+16進-16進 をカンマで並べたもの)。
const unicodeRangePattern =
  /^U\+[0-9A-F]{1,6}(-[0-9A-F]{1,6})?(,U\+[0-9A-F]{1,6}(-[0-9A-F]{1,6})?)*$/;

// parseUnicodeRange は、unicode-range の書き方の文字列を、[始まり, 終わり] の範囲の並びにする。
// 書き方に合わなければ null を返す。そのときは、後のファイルを宣言せず、その字はほかのフォントで描く。
export function parseUnicodeRange(value: unknown): [number, number][] | null {
  if (typeof value !== 'string' || !unicodeRangePattern.test(value)) {
    return null;
  }

  return value.split(',').map((part) => {
    const [start, end] = part.slice(2).split('-');
    const first = Number.parseInt(start!, 16);

    return [first, end === undefined ? first : Number.parseInt(end, 16)];
  });
}

// broadcastExtendedRanges は、後のファイルの字の範囲。範囲の一覧がビルドで作れていなければ null。
const broadcastExtendedRanges = parseUnicodeRange(extendedRange.unicodeRange);

// broadcastExtendedUnicodeRange は、後のファイルの FontFace の unicodeRange の記述子。範囲の一覧がなければ null。
export const broadcastExtendedUnicodeRange =
  broadcastExtendedRanges === null ? null : extendedRange.unicodeRange;

// usesExtendedFont は、text に、後のファイルの字があるかを返す。ranges を省けば、ビルドで作った範囲を使う。
export function usesExtendedFont(text: string, ranges = broadcastExtendedRanges): boolean {
  if (ranges === null) {
    return false;
  }
  for (const char of text) {
    const codepoint = char.codePointAt(0)!;
    // 範囲は小さい順に並ぶので、二分探索で探す。
    let low = 0;
    let high = ranges.length - 1;
    while (low <= high) {
      const middle = (low + high) >> 1;
      const [start, end] = ranges[middle]!;
      if (codepoint < start) {
        high = middle - 1;
      } else if (codepoint > end) {
        low = middle + 1;
      } else {
        return true;
      }
    }
  }

  return false;
}
