// 番組のジャンル (ARIB STD-B10 の content_nibble の大分類) ごとの、番組表の色。

// genreColors は、大分類から、Mantine の色の名前への対応。番組表の枠の背景は、その色の淡い版にする。
const genreColors: Record<number, string> = {
  0: 'blue', // ニュース・報道
  1: 'green', // スポーツ
  2: 'cyan', // 情報・ワイドショー
  3: 'pink', // ドラマ
  4: 'grape', // 音楽
  5: 'yellow', // バラエティ
  6: 'red', // 映画
  7: 'orange', // アニメ・特撮
  8: 'teal', // ドキュメンタリー・教養
  9: 'violet', // 劇場・公演
  10: 'lime', // 趣味・教育
  11: 'indigo', // 福祉
};

// genreBackground は、番組表の枠の背景の色を返す。ジャンルがないか、その他なら、灰色にする。
export function genreBackground(lv1: number | undefined): string {
  const color = lv1 === undefined ? undefined : genreColors[lv1];

  return `var(--mantine-color-${color ?? 'gray'}-light)`;
}

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
