// 番組表ビューのジャンルの条件を、画面の選択肢の値と行き来させる。

// Genre は、ジャンルの条件。lv2 が null なら、大分類のすべてを表す。
export type Genre = { lv1: number; lv2: number | null | undefined };

// encodeGenre は、ジャンルの条件を、選択肢の値 ("7" や "7:0") にする。
export function encodeGenre(genre: Genre): string {
  return genre.lv2 == null ? `${genre.lv1}` : `${genre.lv1}:${genre.lv2}`;
}

// decodeGenre は、選択肢の値を、ジャンルの条件に戻す。
export function decodeGenre(value: string): { lv1: number; lv2: number | null } {
  const [lv1, lv2] = value.split(':');

  return { lv1: Number(lv1), lv2: lv2 === undefined ? null : Number(lv2) };
}
