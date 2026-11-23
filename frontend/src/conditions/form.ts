// 番組の条件の入力と、tenban の番組の条件 (ProgramConditions と ProgramConditionsInput) を行き来させる。
// 番組の検索とルールの編集で、同じ入力を使う。

import type { HighlightKeyword } from '../futon/highlight';
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
  // marks は、番組名の囲み文字の中身 (「新」や「字」など。括弧で書いた [新] なども同じとみなす)。どれか 1 つを持つ番組を対象にする。
  marks: string[];
  // excludeMarks は、どれか 1 つを持つ番組を除外する囲み文字。
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

// promoWords は、宣伝やダイジェストの番組の番組名にある語と、それを見つける形 (#2041)。
// 照合はキーワードを NFKC で正規化して探すので、全角の「ＰＲ」も「PR」で除外できる。PR は、英字の語の一部 (PRIDE、Spring) を
// 宣伝とみなさないよう、前後が英字でないときだけ宣伝の印とみなし、除外も大文字と小文字を区別する。
const promoWords: readonly { text: string; pattern: RegExp; caseSensitive: boolean }[] = [
  {
    text: 'PR',
    pattern: /(?<![A-Za-zＡ-Ｚａ-ｚ])(?:PR|ＰＲ)(?![A-Za-zＡ-Ｚａ-ｚ])/u,
    caseSensitive: true,
  },
  { text: 'ダイジェスト', pattern: /ダイジェスト/u, caseSensitive: false },
];

// conditionsFromProgram は、番組から、ルールを作るときの条件の初期値を作る (設計書 10.3)。
// tenban が番組名から読んだ作品名 (ruleKeyword。話数やサブタイトルを除いたもの。#1240) があればそれを、なければ
// 番組名 (囲み文字を外したもの) をキーワードに、番組のサービスをチャンネルにする。
// 作品名だけをキーワードにすると、宣伝やダイジェストの番組から作ったルールは本編にも当たるが、宣伝の番組そのものも録る。
// 宣伝の番組からルールを作るのは本編を録りたいときなので、番組名に宣伝の語があれば、その語を除外するキーワードに入れる (#2041)。
// 作品名がその語を含むときは、本編まで除外してしまうので入れない。
export function conditionsFromProgram(
  name: string,
  serviceId: string,
  ruleKeyword?: string,
): ConditionsForm {
  const title = ruleKeyword?.trim() || splitProgramMarks(name).name;
  const normalizedTitle = title.normalize('NFKC');
  const excludeKeywords = promoWords
    .filter((word) => word.pattern.test(name) && !normalizedTitle.includes(word.text))
    .map((word) => ({ ...emptyKeyword(), text: word.text, caseSensitive: word.caseSensitive }));

  return {
    ...emptyConditions(),
    keywords: title === '' ? [] : [{ ...emptyKeyword(), text: title }],
    excludeKeywords,
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

// KeywordInputLike は、tenban に送ったキーワード (KeywordInput)。
type KeywordInputLike = {
  readonly text: string;
  readonly regexp?: boolean | null;
  readonly caseSensitive?: boolean | null;
  readonly fields: readonly string[];
};

// nameHighlights は、tenban に送った含めるキーワードのうち、番組名を見るものを、番組名に印を付けるキーワードにする (#966)。
export function nameHighlights(
  keywords: readonly KeywordInputLike[] | null | undefined,
): HighlightKeyword[] {
  return (keywords ?? [])
    .filter((keyword) => keyword.text.trim() !== '' && keyword.fields.includes('NAME'))
    .map((keyword) => ({
      text: keyword.text,
      regexp: keyword.regexp === true,
      caseSensitive: keyword.caseSensitive === true,
    }));
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

// hasRuleTargets は、録画のルールとして番組を選ぶ条件 (含めるキーワード、チャンネル種別、サービス、ジャンル、含める囲み文字) があるかどうかを返す (#1501)。
// 時間帯と曜日、長さ、無料放送、除外の条件は絞り込みとしてだけ効き、それだけのルールは、その時間の全局を録ってディスクを埋めるので、
// 「条件なし」として保存させない。tenban も同じ条件で拒み、照合でも当てない。
export function hasRuleTargets(form: ConditionsForm): boolean {
  const c = toConditionsInput(form);

  return (
    c.keywords.length > 0 ||
    c.channelTypes.length > 0 ||
    c.services.length > 0 ||
    c.genres.length > 0 ||
    c.marks.length > 0
  );
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
