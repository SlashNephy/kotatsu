// 番組のジャンル (ARIB STD-B10 の content_nibble の大分類) の表示名。番組表の色は appearance.ts で決める。

// genreKeys は、大分類の表示名の翻訳キー。添字が大分類の値になる。
const genreKeys = [
  'genre.0',
  'genre.1',
  'genre.2',
  'genre.3',
  'genre.4',
  'genre.5',
  'genre.6',
  'genre.7',
  'genre.8',
  'genre.9',
  'genre.10',
  'genre.11',
  'genre.12',
  'genre.13',
  'genre.14',
  'genre.15',
] as const;

// genreKey は、大分類の表示名の翻訳キーを返す。範囲の外なら、その他にする。
export function genreKey(lv1: number): (typeof genreKeys)[number] {
  return genreKeys[lv1] ?? 'genre.15';
}
