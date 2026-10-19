// 番組の条件の入力と、tenban の番組の条件 (ProgramConditions と ProgramConditionsInput) を行き来させる。
// 番組の検索とルールの編集で、同じ入力を使う。

import { splitProgramMarks } from '../futon/programMarks';

export type KeywordField = 'NAME' | 'DESCRIPTION' | 'EXTENDED';

export const keywordFields: KeywordField[] = ['NAME', 'DESCRIPTION', 'EXTENDED'];

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

export type Genre = { lv1: number; lv2: number | null };

// KeywordMatch は、含めるキーワードの照合のしかた。ALL はすべてに一致、ANY はいずれかに一致。
export type KeywordMatch = 'ALL' | 'ANY';

// ConditionsForm は、番組の条件の入力。
export type ConditionsForm = {
  keywords: KeywordRow[];
  keywordMatch: KeywordMatch;
  excludeKeywords: KeywordRow[];
  channelTypes: string[];
  services: string[];
  // excludeServices は、チャンネル種別やサービスで対象になっても除外するサービス。
  excludeServices: string[];
  genres: Genre[];
  excludeGenres: Genre[];
  timeRanges: TimeRangeRow[];
  // 番組の長さは、tenban と同じく秒で持つ。画面では分で見せるが、入力を変えていなければ秒の値をそのまま送る。
  minDurationSeconds: number | null;
  maxDurationSeconds: number | null;
  freeOnly: boolean;
  // marks は、番組名の印 (囲み文字の中身と、括弧で書いた印。「新」や「字」など)。どれか 1 つを持つ番組を対象にする。
  marks: string[];
  // excludeMarks は、どれか 1 つを持つ番組を除外する印。
  excludeMarks: string[];
};

// KeywordData は、tenban が返すキーワード。
type KeywordData = {
  readonly text: string;
  readonly regexp: boolean;
  readonly caseSensitive: boolean;
  // Relay は、知らない値に備えて、列挙型に "%future added value" を足すので、文字列で受ける。
  readonly fields: readonly string[];
};

// ConditionsData は、tenban が返す番組の条件。
export type ConditionsData = {
  readonly keywords: readonly KeywordData[];
  // Relay は、知らない値に備えて、列挙型に "%future added value" を足すので、文字列で受ける。
  readonly keywordMatch: string;
  readonly excludeKeywords: readonly KeywordData[];
  readonly channelTypes: readonly string[];
  readonly services: readonly { readonly id: string }[];
  readonly excludeServices: readonly { readonly id: string }[];
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
  readonly marks: readonly string[];
  readonly excludeMarks: readonly string[];
};

// emptyKeyword は、キーワードの新しい行。番組名から探す。
export function emptyKeyword(): KeywordRow {
  return { text: '', regexp: false, caseSensitive: false, fields: ['NAME'] };
}

// emptyConditions は、条件の入力の初期値。
export function emptyConditions(): ConditionsForm {
  return {
    keywords: [],
    keywordMatch: 'ALL',
    excludeKeywords: [],
    channelTypes: [],
    services: [],
    excludeServices: [],
    genres: [],
    excludeGenres: [],
    timeRanges: [],
    minDurationSeconds: null,
    maxDurationSeconds: null,
    freeOnly: false,
    marks: [],
    excludeMarks: [],
  };
}

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

// conditionsFromProgram は、番組から、ルールを作るときの条件の初期値を作る (設計書 10.3)。
// 番組名 (囲み文字を外したもの) をキーワードに、番組のサービスをチャンネルにする。
export function conditionsFromProgram(name: string, serviceId: string): ConditionsForm {
  const title = splitProgramMarks(name).name;

  return {
    ...emptyConditions(),
    keywords: title === '' ? [] : [{ ...emptyKeyword(), text: title }],
    services: [serviceId],
  };
}

// fromConditions は、tenban の番組の条件を、入力にする。
export function fromConditions(c: ConditionsData): ConditionsForm {
  return {
    keywords: c.keywords.map(copyKeyword),
    // 知らない照合のしかたは、すべてに一致として扱う。
    keywordMatch: c.keywordMatch === 'ANY' ? 'ANY' : 'ALL',
    excludeKeywords: c.excludeKeywords.map(copyKeyword),
    channelTypes: [...c.channelTypes],
    services: c.services.map((service) => service.id),
    excludeServices: c.excludeServices.map((service) => service.id),
    genres: c.genres.map(copyGenre),
    excludeGenres: c.excludeGenres.map(copyGenre),
    timeRanges: c.timeRanges.map((range) => ({
      weekdays: [...range.weekdays],
      startMinute: range.startMinute,
      durationMinutes: range.durationMinutes,
    })),
    minDurationSeconds: c.minDurationSeconds ?? null,
    maxDurationSeconds: c.maxDurationSeconds ?? null,
    freeOnly: c.freeOnly,
    marks: [...c.marks],
    excludeMarks: [...c.excludeMarks],
  };
}

// toConditionsInput は、入力を、tenban の ProgramConditionsInput にする。
// 中身のないキーワードの行と、曜日のない時間帯の行は、編集の途中とみなして送らない。
export function toConditionsInput(form: ConditionsForm) {
  const keywords = (rows: KeywordRow[]) =>
    rows
      .filter((row) => row.text.trim() !== '' && row.fields.length > 0)
      .map((row) => ({ ...row, text: row.text.trim() }));

  return {
    keywords: keywords(form.keywords),
    keywordMatch: form.keywordMatch,
    excludeKeywords: keywords(form.excludeKeywords),
    channelTypes: form.channelTypes,
    services: form.services,
    excludeServices: form.excludeServices,
    genres: form.genres,
    excludeGenres: form.excludeGenres,
    timeRanges: form.timeRanges.filter((range) => range.weekdays.length > 0),
    minDurationSeconds: form.minDurationSeconds,
    maxDurationSeconds: form.maxDurationSeconds,
    freeOnly: form.freeOnly,
    marks: form.marks,
    excludeMarks: form.excludeMarks,
  };
}

// hasConditions は、番組を絞り込む条件があるかどうかを返す。条件のないルールは、すべての番組を予約してしまう。
export function hasConditions(form: ConditionsForm): boolean {
  const c = toConditionsInput(form);

  return (
    c.keywords.length > 0 ||
    c.excludeKeywords.length > 0 ||
    c.channelTypes.length > 0 ||
    c.services.length > 0 ||
    c.excludeServices.length > 0 ||
    c.genres.length > 0 ||
    c.excludeGenres.length > 0 ||
    c.timeRanges.length > 0 ||
    c.minDurationSeconds != null ||
    c.maxDurationSeconds != null ||
    c.freeOnly ||
    c.marks.length > 0 ||
    c.excludeMarks.length > 0
  );
}
