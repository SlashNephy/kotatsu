// 番組の検索の画面の入力を、番組の条件 (ProgramConditionsInput) に直す。検索とルールの作成で同じものを使う。

// KeywordScope は、キーワードを照合する範囲。
export type KeywordScope = 'name' | 'nameAndDescription' | 'all';

// SearchForm は、検索の画面の入力。
export type SearchForm = {
  keyword: string;
  scope: KeywordScope;
  excludeKeyword: string;
  channelTypes: string[];
  genres: number[];
  freeOnly: boolean;
};

type KeywordField = 'NAME' | 'DESCRIPTION' | 'EXTENDED';

type KeywordInput = {
  text: string;
  regexp: boolean;
  caseSensitive: boolean;
  fields: KeywordField[];
};

// ProgramConditions は、tenban の ProgramConditionsInput。入力のない項目は省き、絞り込まない。
export type ProgramConditions = {
  keywords?: KeywordInput[];
  excludeKeywords?: KeywordInput[];
  channelTypes?: string[];
  genres?: { lv1: number; lv2: null }[];
  freeOnly?: boolean;
};

const scopeFields: Record<KeywordScope, KeywordField[]> = {
  name: ['NAME'],
  nameAndDescription: ['NAME', 'DESCRIPTION'],
  all: ['NAME', 'DESCRIPTION', 'EXTENDED'],
};

// keywords は、空白で区切った語を、それぞれ 1 つのキーワードにする。
function keywords(text: string, fields: KeywordField[]): KeywordInput[] {
  return text
    .split(/\s+/u)
    .filter((word) => word !== '')
    .map((word) => ({ text: word, regexp: false, caseSensitive: false, fields }));
}

// toConditions は、入力を番組の条件にする。
// キーワードは、空白で区切った語のすべてに一致する番組に絞り込む。除外するキーワードは、どれか 1 つに一致したら除く。
export function toConditions(form: SearchForm): ProgramConditions {
  const include = keywords(form.keyword, scopeFields[form.scope]);
  // 除外は、取りこぼしのないよう、番組名だけでなく概要と詳細も見る。
  const exclude = keywords(form.excludeKeyword, scopeFields.all);

  return {
    ...(include.length > 0 ? { keywords: include } : {}),
    ...(exclude.length > 0 ? { excludeKeywords: exclude } : {}),
    ...(form.channelTypes.length > 0 ? { channelTypes: form.channelTypes } : {}),
    ...(form.genres.length > 0 ? { genres: form.genres.map((lv1) => ({ lv1, lv2: null })) } : {}),
    ...(form.freeOnly ? { freeOnly: true } : {}),
  };
}

// isEmpty は、何も絞り込まない条件かどうかを返す。ルールは、すべての番組に一致してしまうので作らせない。
export function isEmpty(conditions: ProgramConditions): boolean {
  return Object.keys(conditions).length === 0;
}
