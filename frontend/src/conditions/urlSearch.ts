// 画面の URL のクエリに載せた、絞り込みの値を読む (#2146)。
// クエリは、共有やブックマークで書き換えられるので、形が違う値と知らない値は破棄する。
// ルーターは、クエリの値を JSON として読むので、配列やオブジェクトもそのまま届く。

import {
  keywordFields,
  toConditionsInput,
  type ConditionsForm,
  type Genre,
  type KeywordField,
  type KeywordRow,
  type TimeRangeRow,
} from './form';

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

// ConditionsSearch は、URL のクエリに載せる番組の条件 (tenban の ProgramConditionsInput と同じ形)。既定の値の項目は載せない。
export type ConditionsSearch = Partial<ReturnType<typeof toConditionsInput>>;

// conditionsSearch は、条件の入力を、URL のクエリに載せる形にする。編集の途中の行と、既定の値の項目は載せない。
// クエリから読み直しても同じになるよう、クエリを読むときと同じ決まりで、形の違う値を破棄し、並びをそろえる。
export function conditionsSearch(form: ConditionsForm): ConditionsSearch {
  const input = toConditionsInput(conditionsFromSearch(toConditionsInput(form)));
  const search: ConditionsSearch = {};
  for (const [key, value] of Object.entries(input) as [keyof ConditionsSearch, unknown][]) {
    if (
      value === null ||
      value === false ||
      (Array.isArray(value) && value.length === 0) ||
      (key === 'keywordMatch' && value === 'ALL')
    ) {
      continue;
    }
    Object.assign(search, { [key]: value });
  }
  // 照合のしかたは、含めるキーワードがあるときだけ意味がある。
  if (search.keywords === undefined) {
    delete search.keywordMatch;
  }

  return search;
}

// conditionsFromSearch は、URL のクエリの番組の条件を、入力にする。形が違う値と知らない値は破棄する。
export function conditionsFromSearch(value: unknown): ConditionsForm {
  const c = typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : {};
  const keywords = parseKeywordRows(c['keywords'], keywordFields);

  return {
    keywords,
    keywordMatch: keywords.length > 0 && c['keywordMatch'] === 'ANY' ? 'ANY' : 'ALL',
    excludeKeywords: parseKeywordRows(c['excludeKeywords'], keywordFields),
    channelTypes: parseStrings(c['channelTypes']),
    services: parseStrings(c['services']),
    excludeServices: parseStrings(c['excludeServices']),
    genres: parseGenres(c['genres']),
    excludeGenres: parseGenres(c['excludeGenres']),
    timeRanges: parseTimeRanges(c['timeRanges']),
    minDurationSeconds: parseSeconds(c['minDurationSeconds']),
    maxDurationSeconds: parseSeconds(c['maxDurationSeconds']),
    freeOnly: c['freeOnly'] === true,
    marks: parseStrings(c['marks']),
    excludeMarks: parseStrings(c['excludeMarks']),
  };
}

// isInteger は、value が min 以上 max 以下の整数かどうか。
function isInteger(value: unknown, min: number, max: number): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= min && value <= max;
}

// parseGenres は、ジャンル (大分類と、あれば中分類。どちらも 0〜15) の配列を読む。
function parseGenres(value: unknown): Genre[] {
  if (!Array.isArray(value)) {
    return [];
  }

  const seen = new Set<string>();

  return value.flatMap((genre: unknown) => {
    if (typeof genre !== 'object' || genre === null) {
      return [];
    }
    const { lv1, lv2 } = genre as Record<string, unknown>;
    if (!isInteger(lv1, 0, 15)) {
      return [];
    }
    const parsed = { lv1, lv2: isInteger(lv2, 0, 15) ? lv2 : null };
    // 同じジャンルは 1 つにまとめる。
    const key = `${parsed.lv1}:${parsed.lv2}`;
    if (seen.has(key)) {
      return [];
    }
    seen.add(key);

    return [parsed];
  });
}

// parseTimeRanges は、曜日と時間帯の行の配列を読む。曜日のない行は、送らないので破棄する。
function parseTimeRanges(value: unknown): TimeRangeRow[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.flatMap((range: unknown) => {
    if (typeof range !== 'object' || range === null) {
      return [];
    }
    const { weekdays, startMinute, durationMinutes } = range as Record<string, unknown>;
    const days = parseChoices(weekdays, [0, 1, 2, 3, 4, 5, 6]);
    if (
      days.length === 0 ||
      !isInteger(startMinute, 0, 24 * 60 - 1) ||
      !isInteger(durationMinutes, 1, 7 * 24 * 60)
    ) {
      return [];
    }

    return [{ weekdays: days, startMinute, durationMinutes }];
  });
}

// maxInt は、GraphQL の Int の上限。
const maxInt = 2 ** 31 - 1;

// parseSeconds は、番組の長さ (秒) を読む。
function parseSeconds(value: unknown): number | null {
  return isInteger(value, 0, maxInt) ? value : null;
}
