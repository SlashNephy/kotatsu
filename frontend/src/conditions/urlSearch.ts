// 画面の URL のクエリに載せた、絞り込みの値を読む (#2146)。
// クエリは、共有やブックマークで書き換えられるので、形が違う値と知らない値は破棄する。
// ルーターは、クエリの値を JSON として読むので、配列やオブジェクトもそのまま届く。

import type { KeywordField, KeywordRow } from './form';

// parseKeywordRows は、キーワードの行の配列を読む。fields は、その画面で選べる項目。
// 語は前後の空白を除く。語のない行と、選べる項目のない行は、送っても当たらないので破棄する。
export function parseKeywordRows(value: unknown, fields: readonly KeywordField[]): KeywordRow[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.flatMap((row: unknown) => {
    if (typeof row !== 'object' || row === null) {
      return [];
    }
    const { text, regexp, caseSensitive, fields: rowFields } = row as Record<string, unknown>;
    if (typeof text !== 'string' || text.trim() === '' || !Array.isArray(rowFields)) {
      return [];
    }
    const picked = fields.filter((field) => rowFields.includes(field));
    if (picked.length === 0) {
      return [];
    }

    return [
      {
        text: text.trim(),
        regexp: regexp === true,
        caseSensitive: caseSensitive === true,
        fields: picked,
      },
    ];
  });
}

// parseStrings は、空でない文字列の配列を読む。同じ値は 1 つにまとめる。
export function parseStrings(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return [
    ...new Set(value.filter((item): item is string => typeof item === 'string' && item !== '')),
  ];
}

// parseChoices は、choices のどれかの値の配列を読む。並びは choices の順にそろえる。
export function parseChoices<T extends string | number>(
  value: unknown,
  choices: readonly T[],
): T[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return choices.filter((choice) => value.includes(choice));
}

// parseDate は、暦にある日付 (YYYY-MM-DD) を読む。暦にない日付と、形の違う値は破棄する。
export function parseDate(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (match === null) {
    return undefined;
  }
  const [year, month, day] = [Number(match[1]), Number(match[2]), Number(match[3])];
  const date = new Date(Date.UTC(year, month - 1, day));

  return date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === day
    ? value
    : undefined;
}
