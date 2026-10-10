// ルールの編集の画面の入力と、tenban のルール (Rule と RuleInput) を行き来させる。
// 画面で編集しない項目 (保存先プールやファイル名の規則、有効期間など) は、そのまま保存し直して失わない。

export type KeywordField = 'NAME' | 'DESCRIPTION' | 'EXTENDED';

const keywordFields: KeywordField[] = ['NAME', 'DESCRIPTION', 'EXTENDED'];

// KeywordRow は、キーワードの 1 行。
export type KeywordRow = {
  text: string;
  regexp: boolean;
  caseSensitive: boolean;
  fields: KeywordField[];
};

// TimeRangeRow は、曜日と時間帯の 1 行。
export type TimeRangeRow = {
  weekdays: number[];
  startMinute: number;
  durationMinutes: number;
};

type Genre = { lv1: number; lv2: number | null };

type Settings = {
  poolId: string | null;
  fileNameTemplate: string | null;
  startMarginSeconds: number | null;
  endMarginSeconds: number | null;
  priority: number | null;
};

// RuleForm は、ルールの編集の画面の入力。
export type RuleForm = {
  name: string;
  memo: string;
  enabled: boolean;
  keywords: KeywordRow[];
  excludeKeywords: KeywordRow[];
  channelTypes: string[];
  services: string[];
  genres: Genre[];
  excludeGenres: Genre[];
  timeRanges: TimeRangeRow[];
  minDurationMinutes: number | null;
  maxDurationMinutes: number | null;
  freeOnly: boolean;
  avoidDuplicates: boolean;
  duplicateWindowDays: number | null;
  followEventRelay: boolean;
  // settings と validFrom、validUntil は、画面では優先度だけを変え、ほかはそのまま保存し直す。
  settings: Settings;
  validFrom: string | null;
  validUntil: string | null;
};

// KeywordData は、tenban が返すキーワード。
type KeywordData = {
  readonly text: string;
  readonly regexp: boolean;
  readonly caseSensitive: boolean;
  // Relay は、知らない値に備えて、列挙型に "%future added value" を足すので、文字列で受ける。
  readonly fields: readonly string[];
};

// RuleData は、tenban が返すルールのうち、編集に使う項目。
export type RuleData = {
  readonly name: string;
  readonly memo: string;
  readonly enabled: boolean;
  readonly conditions: {
    readonly keywords: readonly KeywordData[];
    readonly excludeKeywords: readonly KeywordData[];
    readonly channelTypes: readonly string[];
    readonly services: readonly { readonly id: string }[];
    readonly genres: readonly { readonly lv1: number; readonly lv2?: number | null }[];
    readonly excludeGenres: readonly { readonly lv1: number; readonly lv2?: number | null }[];
    readonly timeRanges: readonly {
      readonly weekdays: readonly number[];
      readonly startMinute: number;
      readonly durationMinutes: number;
    }[];
    readonly minDurationSeconds?: number | null;
    readonly maxDurationSeconds?: number | null;
    readonly freeOnly: boolean;
  };
  readonly settings: {
    readonly poolId?: string | null;
    readonly fileNameTemplate?: string | null;
    readonly startMarginSeconds?: number | null;
    readonly endMarginSeconds?: number | null;
    readonly priority?: number | null;
  };
  readonly validFrom?: string | null;
  readonly validUntil?: string | null;
  readonly avoidDuplicates: boolean;
  readonly duplicateWindowDays?: number | null;
  readonly followEventRelay: boolean;
};

// emptyRuleForm は、新しいルールの入力の初期値。
export function emptyRuleForm(): RuleForm {
  return {
    name: '',
    memo: '',
    enabled: true,
    keywords: [],
    excludeKeywords: [],
    channelTypes: [],
    services: [],
    genres: [],
    excludeGenres: [],
    timeRanges: [],
    minDurationMinutes: null,
    maxDurationMinutes: null,
    freeOnly: false,
    avoidDuplicates: false,
    duplicateWindowDays: null,
    followEventRelay: true,
    settings: {
      poolId: null,
      fileNameTemplate: null,
      startMarginSeconds: null,
      endMarginSeconds: null,
      priority: null,
    },
    validFrom: null,
    validUntil: null,
  };
}

const toMinutes = (seconds: number | null | undefined) =>
  seconds == null ? null : Math.round(seconds / 60);

const copyKeyword = (keyword: KeywordData): KeywordRow => ({
  text: keyword.text,
  regexp: keyword.regexp,
  caseSensitive: keyword.caseSensitive,
  // 知らない項目は、画面で扱えないので落とす。
  fields: keywordFields.filter((field) => keyword.fields.includes(field)),
});

const copyGenre = (genre: { lv1: number; lv2?: number | null }): Genre => ({
  lv1: genre.lv1,
  lv2: genre.lv2 ?? null,
});

// fromRule は、tenban のルールを、編集の画面の入力にする。
export function fromRule(rule: RuleData): RuleForm {
  const c = rule.conditions;

  return {
    name: rule.name,
    memo: rule.memo,
    enabled: rule.enabled,
    keywords: c.keywords.map(copyKeyword),
    excludeKeywords: c.excludeKeywords.map(copyKeyword),
    channelTypes: [...c.channelTypes],
    services: c.services.map((service) => service.id),
    genres: c.genres.map(copyGenre),
    excludeGenres: c.excludeGenres.map(copyGenre),
    timeRanges: c.timeRanges.map((range) => ({
      weekdays: [...range.weekdays],
      startMinute: range.startMinute,
      durationMinutes: range.durationMinutes,
    })),
    minDurationMinutes: toMinutes(c.minDurationSeconds),
    maxDurationMinutes: toMinutes(c.maxDurationSeconds),
    freeOnly: c.freeOnly,
    avoidDuplicates: rule.avoidDuplicates,
    duplicateWindowDays: rule.duplicateWindowDays ?? null,
    followEventRelay: rule.followEventRelay,
    settings: {
      poolId: rule.settings.poolId ?? null,
      fileNameTemplate: rule.settings.fileNameTemplate ?? null,
      startMarginSeconds: rule.settings.startMarginSeconds ?? null,
      endMarginSeconds: rule.settings.endMarginSeconds ?? null,
      priority: rule.settings.priority ?? null,
    },
    validFrom: rule.validFrom ?? null,
    validUntil: rule.validUntil ?? null,
  };
}

// toInput は、編集の画面の入力を、tenban の RuleInput にする。
// 中身のないキーワードの行と、曜日のない時間帯の行は、編集の途中とみなして送らない。
export function toInput(form: RuleForm) {
  const keywords = (rows: KeywordRow[]) =>
    rows
      .filter((row) => row.text.trim() !== '' && row.fields.length > 0)
      .map((row) => ({ ...row, text: row.text.trim() }));

  return {
    name: form.name.trim(),
    memo: form.memo,
    enabled: form.enabled,
    conditions: {
      keywords: keywords(form.keywords),
      excludeKeywords: keywords(form.excludeKeywords),
      channelTypes: form.channelTypes,
      services: form.services,
      genres: form.genres,
      excludeGenres: form.excludeGenres,
      timeRanges: form.timeRanges.filter((range) => range.weekdays.length > 0),
      minDurationSeconds: form.minDurationMinutes == null ? null : form.minDurationMinutes * 60,
      maxDurationSeconds: form.maxDurationMinutes == null ? null : form.maxDurationMinutes * 60,
      freeOnly: form.freeOnly,
    },
    settings: form.settings,
    validFrom: form.validFrom,
    validUntil: form.validUntil,
    avoidDuplicates: form.avoidDuplicates,
    duplicateWindowDays: form.avoidDuplicates ? form.duplicateWindowDays : null,
    followEventRelay: form.followEventRelay,
  };
}

// hasConditions は、番組を絞り込む条件があるかどうかを返す。条件のないルールは、すべての番組を予約してしまう。
export function hasConditions(form: RuleForm): boolean {
  const c = toInput(form).conditions;

  return (
    c.keywords.length > 0 ||
    c.excludeKeywords.length > 0 ||
    c.channelTypes.length > 0 ||
    c.services.length > 0 ||
    c.genres.length > 0 ||
    c.excludeGenres.length > 0 ||
    c.timeRanges.length > 0 ||
    c.minDurationSeconds != null ||
    c.maxDurationSeconds != null ||
    c.freeOnly
  );
}
