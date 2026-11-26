// 番組検索 (設計書 10.3) の、URL のクエリ (#2146)。

import { emptyConditions, emptyKeyword, type ConditionsForm } from '../conditions/form';
import {
  conditionsFromSearch,
  conditionsSearch,
  type ConditionsSearch,
} from '../conditions/urlSearch';

// ProgramSearchSearch は、番組検索の URL のクエリ (#2146)。番組の詳細から戻ったときや、共有やブックマークで開き直したときに、
// 同じ条件で探す。ヘッダーの検索 (#1627) と、過去の番組の詳細から同じ番組名で探すとき (#1121) も、これで条件を渡す。
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
    ...(typeof archive === 'number' && Number.isFinite(archive)
      ? { archive: archiveParam(archive) }
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
