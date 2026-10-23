// ジャンルの条件の選択肢。大分類 (アニメ・特撮のすべて) と中分類 (映画のアニメ) を混ぜて選べるようにする。
// 照合は、どれか 1 つに一致すれば対象にし、中分類を省いた条件は大分類のすべてに一致する (match.anyGenre)。

import type { ComboboxItem, ComboboxItemGroup } from '@mantine/core';
import type { TFunction } from 'i18next';

import { resources } from '../i18n';
import { genreKey, isGenre } from './genres';
import { decodeGenre, encodeGenre, type Genre } from './guideView';

type SubGenres = (typeof resources)['ja']['translation']['subGenre'];
type SubGenreKey = {
  [L in keyof SubGenres]: `subGenre.${L}.${keyof SubGenres[L] & string}`;
}[keyof SubGenres];

// selectableLv1s は、選べる大分類。予備 (12、13) と拡張 (14) は、番組に付かないので出さない。
const selectableLv1s = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 15];
// otherLv1 は、大分類の「その他」。中分類も「その他」しかないので、中分類は出さない。
const otherLv1 = 15;
// subGenreNames は、大分類ごとの、中分類の値から表示名への対応。
const subGenreNames: Record<string, Record<string, string> | undefined> =
  resources.ja.translation.subGenre;

// subGenreKey は、中分類の表示名の翻訳キーを返す。ARIB STD-B10 に名前のない組なら undefined を返す。
export function subGenreKey(lv1: number, lv2: number): SubGenreKey | undefined {
  return subGenreNames[lv1]?.[lv2] === undefined
    ? undefined
    : (`subGenre.${lv1}.${lv2}` as SubGenreKey);
}

// genreLabel は、選んだジャンルの表示名。中分類は、どの大分類のものか分かるよう、大分類の名前を前に付ける。
export function genreLabel(t: TFunction, genre: Genre): string {
  const lv1 = t(genreKey(genre.lv1));
  if (genre.lv2 == null) {
    return lv1;
  }
  const key = subGenreKey(genre.lv1, genre.lv2);

  return key === undefined
    ? t('guide.view.subGenre', { genre: lv1, lv2: genre.lv2 })
    : `${lv1}／${t(key)}`;
}

// programGenreLabels は、番組や録画に付いたジャンルの表示名を、「大分類／中分類」で並べて返す。
// ジャンルでない拡張などは除き、同じ名前は 1 つにまとめる。
export function programGenreLabels(
  t: TFunction,
  genres: readonly { readonly lv1: number; readonly lv2: number }[],
): string[] {
  return [
    ...new Set(
      genres
        .filter((genre) => isGenre(genre.lv1))
        .map((genre) => genreLabel(t, { lv1: genre.lv1, lv2: genre.lv2 })),
    ),
  ];
}

// genreOptions は、ジャンルのドロップダウンの選択肢を、大分類ごとのグループにして返す。
// グループの先頭は大分類のすべてで、続けて中分類を並べる。
// selected は、選んでいるジャンル。名前のない中分類 (取り込んだルールにあるものなど) も、消すまで選択肢に残す。
export function genreOptions(
  t: TFunction,
  selected: readonly Genre[],
): ComboboxItemGroup<ComboboxItem>[] {
  const genres: Genre[] = [];
  for (const lv1 of selectableLv1s) {
    genres.push({ lv1, lv2: null });
    if (lv1 === otherLv1) {
      continue;
    }
    for (const lv2 of Object.keys(subGenreNames[lv1] ?? {})) {
      genres.push({ lv1, lv2: Number(lv2) });
    }
  }
  const values = genres.map(encodeGenre);
  for (const genre of selected) {
    const value = encodeGenre(genre);
    if (!values.includes(value)) {
      values.push(value);
    }
  }

  const groups = new Map<number, ComboboxItemGroup<ComboboxItem>>();
  for (const value of values) {
    const genre = decodeGenre(value);
    let group = groups.get(genre.lv1);
    if (group === undefined) {
      group = { group: t(genreKey(genre.lv1)), items: [] };
      groups.set(genre.lv1, group);
    }
    group.items.push({ value, label: genreLabel(t, genre) });
  }

  return [...groups.values()];
}
