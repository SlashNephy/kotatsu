import { containsAllWords, normalizeForSearch, searchWords } from '../futon/searchText';
import { outOfValidity } from './validity';

// ルールの一覧 (設計書 10.3、#1493) を、画面の中で探し、絞り、並べ替える。
// ルールは数百件までの見込みなので、すべてを読んでから画面で絞る。

// RuleStateFilter は、ルールの状態での絞り込み。outOfValidity は、有効だが有効期間の外のルール (一覧の「期間外」の印と同じ、#464)。
export type RuleStateFilter = 'enabled' | 'disabled' | 'outOfValidity';

// ruleStateFilters は、状態の絞り込みの並び。
export const ruleStateFilters: readonly RuleStateFilter[] = [
  'enabled',
  'disabled',
  'outOfValidity',
];

// RuleReservationFilter は、これからの予約があるかどうかでの絞り込み。none は、条件に当たる放送がないルールを探すのに使う。
export type RuleReservationFilter = 'some' | 'none';

export const ruleReservationFilters: readonly RuleReservationFilter[] = ['some', 'none'];

// RuleSortKey は、並べ替えの鍵。created (作った順) が既定で、これまでの一覧と同じ並び。
export type RuleSortKey = 'created' | 'name' | 'updated' | 'reservations';

export const ruleSortKeys: readonly RuleSortKey[] = ['created', 'name', 'updated', 'reservations'];

// RuleListSearch は、ルールの一覧の URL のクエリのうち、探す、絞る、並べ替えるもの。既定の値は載せない。
export type RuleListSearch = {
  // q は、ルールの名前、メモ、キーワードから探す語。空白で区切ると、すべてを含むルールに絞る。
  q?: string;
  state?: RuleStateFilter;
  reservations?: RuleReservationFilter;
  // sort は、並べ替えの鍵。既定 (created) は載せない。
  sort?: Exclude<RuleSortKey, 'created'>;
  // order は、並べ替えの向き。既定 (昇順) は載せない。
  order?: 'desc';
};

// parseRuleListSearch は、URL のクエリを読む。知らない値と、既定の値は捨てる。
export function parseRuleListSearch(search: Record<string, unknown>): RuleListSearch {
  const rawQuery = search['q'];
  // 数字だけの語は、ルーターが数として読むので、文字列に戻す。
  const q =
    typeof rawQuery === 'string' || typeof rawQuery === 'number' ? String(rawQuery) : undefined;
  const state = ruleStateFilters.find((value) => value === search['state']);
  const reservations = ruleReservationFilters.find((value) => value === search['reservations']);
  const sort = ruleSortKeys.find((value) => value !== 'created' && value === search['sort']);

  return {
    ...(q !== undefined && q.trim() !== '' ? { q } : {}),
    ...(state === undefined ? {} : { state }),
    ...(reservations === undefined ? {} : { reservations }),
    ...(sort === undefined || sort === 'created' ? {} : { sort }),
    ...(search['order'] === 'desc' ? { order: 'desc' as const } : {}),
  };
}

// isFiltered は、探す語か絞り込みがあり、一覧の一部だけを出しているかもしれないかどうか。並べ替えは数えない。
export function isFiltered(search: RuleListSearch): boolean {
  return search.q !== undefined || search.state !== undefined || search.reservations !== undefined;
}

// ListedRule は、探す、絞る、並べ替えるのに使うルールの項目。時刻は RFC 3339。
export type ListedRule = {
  readonly id: string;
  readonly name: string;
  readonly memo: string;
  readonly enabled: boolean;
  readonly conditions: { readonly keywords: readonly { readonly text: string }[] };
  readonly validFrom: string | null | undefined;
  readonly validUntil: string | null | undefined;
  readonly createdAt: string;
  readonly updatedAt: string;
  readonly upcomingReservationCount: number;
};

const nameCollator = new Intl.Collator('ja', { numeric: true });

// compareBy は、鍵ごとの、昇順の比べ方。
const compareBy: Record<RuleSortKey, (a: ListedRule, b: ListedRule) => number> = {
  created: (a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt),
  name: (a, b) => nameCollator.compare(a.name, b.name),
  updated: (a, b) => Date.parse(a.updatedAt) - Date.parse(b.updatedAt),
  reservations: (a, b) => a.upcomingReservationCount - b.upcomingReservationCount,
};

// matchesState は、ルールが状態の絞り込みに当たるかどうか。
function matchesState(rule: ListedRule, state: RuleStateFilter, now: number): boolean {
  switch (state) {
    case 'enabled':
      return rule.enabled;
    case 'disabled':
      return !rule.enabled;
    case 'outOfValidity':
      return rule.enabled && outOfValidity(rule.validFrom, rule.validUntil, now);
  }
}

// visibleRules は、rules を、search で探して絞り、並べ替えた一覧を返す。now は、有効期間の外かを決める時刻 (ミリ秒)。
// rules は、サーバーの並び (作った順) で渡す。鍵が同じルールは、その並びのまま残す。
export function visibleRules<R extends ListedRule>(
  rules: readonly R[],
  search: RuleListSearch,
  now: number,
): R[] {
  const words = searchWords(search.q ?? '');
  const filtered = rules.filter(
    (rule) =>
      (search.state === undefined || matchesState(rule, search.state, now)) &&
      (search.reservations === undefined ||
        (search.reservations === 'some') === rule.upcomingReservationCount > 0) &&
      containsAllWords(
        normalizeForSearch(
          [rule.name, rule.memo, ...rule.conditions.keywords.map((keyword) => keyword.text)].join(
            '\n',
          ),
        ),
        words,
      ),
  );
  const compare = compareBy[search.sort ?? 'created'];
  const sign = search.order === 'desc' ? -1 : 1;

  // Array.prototype.sort は安定なので、鍵が同じルールは渡された並びのまま残る。
  return filtered.sort((a, b) => sign * compare(a, b));
}
