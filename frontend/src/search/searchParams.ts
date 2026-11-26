// 番組検索 (設計書 10.3) の、URL のクエリ (#2146)。

import { emptyConditions, emptyKeyword, type ConditionsForm } from '../conditions/form';
import {
  conditionsFromSearch,
  conditionsSearch,
  type ConditionsSearch,
} from '../conditions/urlSearch';

// ProgramSearchSearch は、番組検索の URL のクエリ (#2146)。番組の詳細から戻ったときや、共有やブックマークで開き直したときに、
// 同じ条件で探す。ヘッダーの検索 (#1627) と、過去の番組の詳細から同じ番組名で探すとき (#1121) も、これで条件を渡す。
declare module '@tanstack/react-router' {
  interface HistoryState {
    // searchSync は、番組検索の画面が、自分の入力から URL を書き換えた履歴の項目かどうか。
    // 画面の外から移ってきた項目では、入力を URL の条件で始め直す。
    searchSync?: true;
    // searchStart は、画面の外から番組検索を始めた時刻。同じ URL で探し直したときも、ルーターは同じ URL と状態の遷移を
    // 履歴に積まないので、時刻を変えて履歴の項目を分け、入力を始め直す。
    searchStart?: number;
  }
}

// startSearchState は、画面の外から番組検索を始めるときの、遷移の状態。
export function startSearchState(): { searchStart: number } {
  return { searchStart: Date.now() };
}

export type ProgramSearchSearch = {
  conditions?: ConditionsSearch;
  // archive は、過去の放送も含めて探すときの、さかのぼる日数。なければ、これから放送する番組だけを探す。
  archive?: number;
  // program は、開いている番組の詳細の ID。
  program?: string;
};

// defaultArchiveDays は、過去の放送を含めるときに、初めにさかのぼる日数。
export const defaultArchiveDays = 30;

// maxArchiveDays は、過去の放送をさかのぼれる日数の上限。
const maxArchiveDays = 3650;

// archiveParam は、さかのぼる日数を、クエリに載せる値 (1〜maxArchiveDays の整数) にする。
export function archiveParam(days: number): number {
  return Math.min(maxArchiveDays, Math.max(1, Math.round(days)));
}

// parseProgramSearch は、URL のクエリを読む。形が違う値と知らない値は破棄する。
export function parseProgramSearch(search: Record<string, unknown>): ProgramSearchSearch {
  const conditions = conditionsSearch(conditionsFromSearch(search['conditions']));
  const archive = search['archive'];
  const program = search['program'];

  return {
    ...(Object.keys(conditions).length > 0 ? { conditions } : {}),
    ...(typeof archive === 'number' &&
    Number.isInteger(archive) &&
    archive >= 1 &&
    archive <= maxArchiveDays
      ? { archive }
      : {}),
    ...(typeof program === 'string' && program !== '' ? { program } : {}),
  };
}

// formFromSearch は、クエリの条件から、入力を作る。条件がなければ、キーワードの行を 1 つ開けておき、すぐに打ち始められるようにする。
export function formFromSearch(conditions: ConditionsSearch | undefined): ConditionsForm {
  return conditions === undefined
    ? { ...emptyConditions(), keywords: [emptyKeyword()] }
    : conditionsFromSearch(conditions);
}
