// 録画の一覧 (設計書 10.3) の、絞り込みと並び順を URL のクエリに持つ (#2146)。
// 詳細から戻ったときや、共有やブックマークで開き直したときに、同じ一覧を出す。既定の値は載せない。

import { useLocation } from '@tanstack/react-router';

import type { KeywordField, KeywordMatch, KeywordRow } from '../conditions/form';
import { parseChoices, parseDate, parseKeywordRows, parseStrings } from '../conditions/urlSearch';
import { broadcastRange } from './filterRange';
import { recordingStates } from './states';

type RecordingState = (typeof recordingStates)[number];

// recordingKeywordFields は、録画の検索で選べるキーワードの項目。録画は詳細では探さない (設計書 3.5)。
export const recordingKeywordFields: readonly KeywordField[] = ['NAME', 'DESCRIPTION'];

// CaptionKeyword は、字幕の文で探すキーワード (#2215)。字幕は 1 つの文なので、照合する項目を持たない。
export type CaptionKeyword = { text: string; regexp: boolean; caseSensitive: boolean };

// recordingGenres は、絞り込みで選べるジャンルの大分類 (ニュース・報道〜福祉と、その他)。
export const recordingGenres: readonly number[] = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 15];

// RecordingListSearch は、録画の一覧の URL のクエリ。
export type RecordingListSearch = {
  // rule は、このルールで録った録画だけを出すときの、ルールの ID (#468)。
  rule?: string;
  keywords?: KeywordRow[];
  // keywordMatch は、含めるキーワードの照合のしかた。既定 (ALL) は載せない。
  keywordMatch?: 'ANY';
  excludeKeywords?: KeywordRow[];
  // captionKeywords は、字幕の文で探すキーワード。captionKeywordMatch は、その照合のしかたで、既定 (ALL) は載せない (#2215)。
  captionKeywords?: CaptionKeyword[];
  captionKeywordMatch?: 'ANY';
  tags?: string[];
  states?: RecordingState[];
  services?: string[];
  genres?: number[];
  // from と to は、放送日の期間 (YYYY-MM-DD)。両端を含む。
  from?: string;
  to?: string;
  // order は、並び順。既定 (新しい順) は載せない。
  order?: 'oldest';
};

// parseRecordingListSearch は、URL のクエリを読む。知らない値と、既定の値は破棄する。
export function parseRecordingListSearch(search: Record<string, unknown>): RecordingListSearch {
  const rule = search['rule'];
  const keywords = parseKeywordRows(search['keywords'], recordingKeywordFields);
  const excludeKeywords = parseKeywordRows(search['excludeKeywords'], recordingKeywordFields);
  const captionKeywords = parseCaptionKeywords(search['captionKeywords']);
  const tags = parseStrings(search['tags']);
  const states = parseChoices(search['states'], recordingStates);
  const services = parseStrings(search['services']);
  const genres = parseChoices(search['genres'], recordingGenres);
  const from = parseDate(search['from']);
  const to = parseDate(search['to']);

  return {
    ...(typeof rule === 'string' && rule !== '' ? { rule } : {}),
    ...(keywords.length > 0 ? { keywords } : {}),
    // 照合のしかたは、含めるキーワードがあるときだけ意味がある。
    ...(keywords.length > 0 && search['keywordMatch'] === 'ANY'
      ? { keywordMatch: 'ANY' as const }
      : {}),
    ...(excludeKeywords.length > 0 ? { excludeKeywords } : {}),
    ...(captionKeywords.length > 0 ? { captionKeywords } : {}),
    ...(captionKeywords.length > 0 && search['captionKeywordMatch'] === 'ANY'
      ? { captionKeywordMatch: 'ANY' as const }
      : {}),
    ...(tags.length > 0 ? { tags } : {}),
    ...(states.length > 0 ? { states } : {}),
    ...(services.length > 0 ? { services } : {}),
    ...(genres.length > 0 ? { genres } : {}),
    ...(from === undefined ? {} : { from }),
    ...(to === undefined ? {} : { to }),
    ...(search['order'] === 'oldest' ? { order: 'oldest' as const } : {}),
  };
}

// parseCaptionKeywords は、字幕の文で探すキーワードの配列を読む。語は前後の空白を除き、語のない行は破棄する。
function parseCaptionKeywords(value: unknown): CaptionKeyword[] {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.flatMap((row: unknown) => {
    if (typeof row !== 'object' || row === null) {
      return [];
    }
    const { text, regexp, caseSensitive } = row as Record<string, unknown>;
    if (typeof text !== 'string' || text.trim() === '') {
      return [];
    }

    return [{ text: text.trim(), regexp: regexp === true, caseSensitive: caseSensitive === true }];
  });
}

// RecordingFilterInput は、tenban の RecordingFilter のうち、一覧で使う項目。
export type RecordingFilterInput = {
  keywords?: KeywordRow[];
  keywordMatch?: KeywordMatch;
  excludeKeywords?: KeywordRow[];
  captionKeywords?: CaptionKeyword[];
  captionKeywordMatch?: KeywordMatch;
  tagIds?: string[];
  states?: RecordingState[];
  serviceIds?: string[];
  genres?: number[];
  since?: string;
  until?: string;
  ruleId?: string;
};

// recordingFilter は、クエリを tenban に送る絞り込みにする。並び順は含めない。
export function recordingFilter(search: RecordingListSearch): RecordingFilterInput {
  const keywords = search.keywords ?? [];
  const excludeKeywords = search.excludeKeywords ?? [];

  return {
    ...(keywords.length > 0 || excludeKeywords.length > 0
      ? { keywords, keywordMatch: search.keywordMatch ?? 'ALL', excludeKeywords }
      : {}),
    // 字幕の照合のしかたは、省くと ALL になるので、ANY のときだけ送る。
    ...(search.captionKeywords === undefined
      ? {}
      : {
          captionKeywords: search.captionKeywords,
          ...(search.captionKeywordMatch === undefined
            ? {}
            : { captionKeywordMatch: search.captionKeywordMatch }),
        }),
    ...(search.tags === undefined ? {} : { tagIds: search.tags }),
    ...(search.states === undefined ? {} : { states: search.states }),
    ...(search.services === undefined ? {} : { serviceIds: search.services }),
    ...(search.genres === undefined ? {} : { genres: search.genres }),
    ...broadcastRange(search.from ?? '', search.to ?? ''),
    ...(search.rule === undefined ? {} : { ruleId: search.rule }),
  };
}

// activeFilterCount は、使っている絞り込みの条件の数。閉じた絞り込みのボタンに出す。
// 照合のしかたは条件に添えるものなので数えず、空の配列 (除外するキーワードのない行など) も数えない。
export function activeFilterCount(filter: object): number {
  return Object.entries(filter).filter(
    ([key, value]) =>
      key !== 'keywordMatch' &&
      key !== 'captionKeywordMatch' &&
      !(Array.isArray(value) && value.length === 0),
  ).length;
}

// RecordingFilterForm は、絞り込みのフォームの入力。決めるまでは、クエリを変えない。
export type RecordingFilterForm = {
  keywords: KeywordRow[];
  keywordMatch: KeywordMatch;
  excludeKeywords: KeywordRow[];
  // captionKeywords は、字幕の文で探すキーワードの行。行の入力を番組名のキーワードと共用するので、項目は空にする。
  captionKeywords: KeywordRow[];
  captionKeywordMatch: KeywordMatch;
  tags: string[];
  states: string[];
  services: string[];
  // genres は、MultiSelect で選ぶので、文字列で持つ。
  genres: string[];
  from: string;
  to: string;
};

// recordingKeyword は、録画の検索のキーワードの行の初めの値。番組名と概要から探す。
export function recordingKeyword(): KeywordRow {
  return { text: '', regexp: false, caseSensitive: false, fields: ['NAME', 'DESCRIPTION'] };
}

// captionKeyword は、字幕の文で探すキーワードの行の初めの値。字幕は照合する項目を持たない。
export function captionKeyword(): KeywordRow {
  return { text: '', regexp: false, caseSensitive: false, fields: [] };
}

// formFromSearch は、クエリからフォームの入力を作る。含めるキーワードと字幕のキーワードがなければ、空の行を 1 つ開けておく。
export function formFromSearch(search: RecordingListSearch): RecordingFilterForm {
  return {
    keywords: search.keywords ?? [recordingKeyword()],
    keywordMatch: search.keywordMatch ?? 'ALL',
    excludeKeywords: search.excludeKeywords ?? [],
    captionKeywords: search.captionKeywords?.map((keyword) => ({ ...keyword, fields: [] })) ?? [
      captionKeyword(),
    ],
    captionKeywordMatch: search.captionKeywordMatch ?? 'ALL',
    tags: search.tags ?? [],
    states: search.states ?? [],
    services: search.services ?? [],
    genres: (search.genres ?? []).map(String),
    from: search.from ?? '',
    to: search.to ?? '',
  };
}

// searchFromForm は、フォームの入力をクエリにする。ルールと並び順は、フォームの外なので、今のクエリから引き継ぐ。
export function searchFromForm(
  form: RecordingFilterForm,
  current: RecordingListSearch,
): RecordingListSearch {
  return parseRecordingListSearch({
    rule: current.rule,
    order: current.order,
    keywords: form.keywords,
    keywordMatch: form.keywordMatch,
    excludeKeywords: form.excludeKeywords,
    captionKeywords: form.captionKeywords,
    captionKeywordMatch: form.captionKeywordMatch,
    tags: form.tags,
    states: form.states,
    services: form.services,
    genres: form.genres.map(Number),
    from: form.from,
    to: form.to,
  });
}

declare module '@tanstack/react-router' {
  interface HistoryState {
    // recordingsSearch は、録画の詳細を開いた一覧のクエリ。詳細の「戻る」で、同じ絞り込みの一覧に戻る (#2146)。
    recordingsSearch?: RecordingListSearch;
  }
}

// useListState は、録画の詳細と再生の間で引き継ぐ、開いた一覧のクエリの履歴の状態。一覧から開いていなければ undefined。
export function useListState(): { recordingsSearch: RecordingListSearch } | undefined {
  const listSearch = useLocation({ select: (location) => location.state.recordingsSearch });

  return listSearch === undefined ? undefined : { recordingsSearch: listSearch };
}
