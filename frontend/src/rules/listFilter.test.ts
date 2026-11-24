import { describe, expect, test } from 'vitest';

import {
  isFiltered,
  parseRuleListSearch,
  ruleSections,
  sectionKey,
  visibleRules,
  type ListedRule,
  type RuleListSearch,
} from './listFilter';

const now = Date.parse('2026-11-16T00:00:00Z');

// rule は、テストのルールを作る。指定しない項目は、有効で、期間を区切らず、予約のないルールにする。
function rule(fields: Partial<ListedRule> & { id: string; keywords?: string[] }): ListedRule {
  const { keywords = [], ...rest } = fields;

  return {
    name: fields.id,
    memo: '',
    enabled: true,
    conditions: { keywords: keywords.map((text) => ({ text })) },
    validFrom: null,
    validUntil: null,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    upcomingReservationCount: 0,
    ...rest,
  };
}

// サーバーの並び (作った順) で並べたルール。
const rules: readonly ListedRule[] = [
  rule({
    id: 'anime',
    name: 'ＳＰＹ×ＦＡＭＩＬＹ',
    memo: '日曜の夕方',
    keywords: ['スパイファミリー'],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-11-01T00:00:00Z',
    upcomingReservationCount: 3,
  }),
  rule({
    id: 'news',
    name: 'ニュース',
    memo: 'NHK の朝',
    keywords: ['おはよう日本'],
    createdAt: '2026-02-01T00:00:00Z',
    updatedAt: '2026-03-01T00:00:00Z',
    upcomingReservationCount: 10,
  }),
  rule({
    id: 'ended',
    name: 'アニメ 2026 夏',
    keywords: ['ｶﾀｶﾅ番組'],
    createdAt: '2026-03-01T00:00:00Z',
    updatedAt: '2026-08-01T00:00:00Z',
  }),
  rule({
    id: 'disabled',
    name: 'アニメ 2026 秋',
    enabled: false,
    createdAt: '2026-04-01T00:00:00Z',
    updatedAt: '2026-04-01T00:00:00Z',
    upcomingReservationCount: 3,
  }),
  rule({
    id: 'expired',
    name: 'ドラマ',
    validUntil: '2026-10-01T00:00:00Z',
    createdAt: '2026-05-01T00:00:00Z',
    updatedAt: '2026-05-01T00:00:00Z',
  }),
  rule({
    id: 'expiredDisabled',
    name: 'ドラマ (旧)',
    enabled: false,
    validUntil: '2026-10-01T00:00:00Z',
    createdAt: '2026-06-01T00:00:00Z',
    updatedAt: '2026-06-01T00:00:00Z',
  }),
  rule({
    id: 'notYet',
    name: '来年の特番',
    validFrom: '2027-01-01T00:00:00Z',
    createdAt: '2026-07-01T00:00:00Z',
    updatedAt: '2026-07-01T00:00:00Z',
  }),
];

const ids = (search: RuleListSearch) => visibleRules(rules, search, now).map((item) => item.id);

describe('visibleRules', () => {
  test.each<[string, RuleListSearch, string[]]>([
    [
      '何も指定しなければ、サーバーの並びのまま全部',
      {},
      ['anime', 'news', 'ended', 'disabled', 'expired', 'expiredDisabled', 'notYet'],
    ],
    ['半角で打った語で、全角の名前に当たる', { q: 'spy' }, ['anime']],
    ['全角で打った語で、半角のメモに当たる', { q: 'ｎｈｋ' }, ['news']],
    ['キーワードの語にも当たる', { q: 'おはよう' }, ['news']],
    ['半角のカタカナのキーワードに、全角で打った語で当たる', { q: 'カタカナ' }, ['ended']],
    ['空白で区切った語は、すべてを含むものだけ', { q: 'アニメ　秋' }, ['disabled']],
    ['名前とメモをまたいで、すべての語を含めば当たる', { q: 'family 夕方' }, ['anime']],
    ['どれにも当たらない語', { q: 'ない語' }, []],
    [
      '有効だけ (期間外も有効)',
      { state: 'enabled' },
      ['anime', 'news', 'ended', 'expired', 'notYet'],
    ],
    ['無効だけ', { state: 'disabled' }, ['disabled', 'expiredDisabled']],
    // 一覧の「期間外」のマークは、有効なルールにだけ出すので、無効で期間の切れたルールは含めない。
    [
      '期間外は、有効で、期間が切れたか、まだ始まっていないもの',
      { state: 'outOfValidity' },
      ['expired', 'notYet'],
    ],
    ['これからの予約があるもの', { reservations: 'some' }, ['anime', 'news', 'disabled']],
    [
      '有効で、これからの予約がないもの',
      { state: 'enabled', reservations: 'none' },
      ['ended', 'expired', 'notYet'],
    ],
    // 日本語の辞書の順で、英字、かな (五十音)、漢字の順に並べる。
    [
      '名前の順',
      { sort: 'name' },
      ['anime', 'ended', 'disabled', 'expired', 'expiredDisabled', 'news', 'notYet'],
    ],
    [
      '作った日の新しい順',
      { order: 'desc' },
      ['notYet', 'expiredDisabled', 'expired', 'disabled', 'ended', 'news', 'anime'],
    ],
    [
      '変えた日の新しい順',
      { sort: 'updated', order: 'desc' },
      ['anime', 'ended', 'notYet', 'expiredDisabled', 'expired', 'disabled', 'news'],
    ],
    // 件数が同じルールは、作った順のまま並べる。
    [
      'これからの予約の多い順',
      { sort: 'reservations', order: 'desc' },
      ['news', 'anime', 'disabled', 'ended', 'expired', 'expiredDisabled', 'notYet'],
    ],
    [
      'これからの予約の少ない順',
      { sort: 'reservations' },
      ['ended', 'expired', 'expiredDisabled', 'notYet', 'anime', 'disabled', 'news'],
    ],
    [
      '絞り込みと並べ替えを合わせる',
      { q: 'アニメ', sort: 'reservations', order: 'desc' },
      ['disabled', 'ended'],
    ],
  ])('%s', (_, search, want) => {
    expect(ids(search)).toEqual(want);
  });

  test('渡した一覧は並べ替えない', () => {
    const before = rules.map((item) => item.id);
    visibleRules(rules, { sort: 'name' }, now);
    expect(rules.map((item) => item.id)).toEqual(before);
  });
});

describe('parseRuleListSearch', () => {
  test.each<[string, Record<string, unknown>, RuleListSearch]>([
    ['何もなければ空', {}, {}],
    [
      '知っている値はそのまま読む',
      {
        q: 'アニメ',
        state: 'outOfValidity',
        reservations: 'none',
        sort: 'reservations',
        order: 'desc',
      },
      {
        q: 'アニメ',
        state: 'outOfValidity',
        reservations: 'none',
        sort: 'reservations',
        order: 'desc',
      },
    ],
    // ルーターは、数字だけの値を数として読む。
    ['数として読んだ語は、文字列に戻す', { q: 2026 }, { q: '2026' }],
    ['空白だけの語は捨てる', { q: '　 ' }, {}],
    ['既定の並べ替えと向きは載せない', { sort: 'created', order: 'asc' }, {}],
    [
      '知らない値は捨てる',
      { q: true, state: 'ENABLED', reservations: 1, sort: 'random', order: 'up', page: 2 },
      {},
    ],
  ])('%s', (_, search, want) => {
    expect(parseRuleListSearch(search)).toEqual(want);
  });
});

describe('isFiltered', () => {
  test.each<[RuleListSearch, boolean]>([
    [{}, false],
    [{ sort: 'name', order: 'desc' }, false],
    [{ q: 'a' }, true],
    [{ state: 'disabled' }, true],
    [{ reservations: 'none' }, true],
  ])('%j は %s', (search, want) => {
    expect(isFiltered(search)).toBe(want);
  });
});

describe('ruleSections', () => {
  // groups は、サーバーが並べる順 (アニメ、ドラマ、空) のグループ。
  const groups = [
    { id: 'g-anime', name: 'アニメ' },
    { id: 'g-drama', name: 'ドラマ' },
    { id: 'g-empty', name: '空' },
  ];
  const grouped = (id: string, group: string | null) => ({
    id,
    group: group === null ? null : { id: group },
  });
  // 並べ替えた後のルール。見出しの中でも、この並びを保つ。
  const sorted = [
    grouped('b', 'g-drama'),
    grouped('a', null),
    grouped('c', 'g-anime'),
    grouped('d', 'g-drama'),
    grouped('e', 'g-gone'),
  ];
  const layout = (sections: ReturnType<typeof ruleSections>) =>
    sections.map((section) => [
      section.group?.name ?? 'その他',
      section.rules.map((item) => item.id),
    ]);

  test.each<[string, boolean, (string | string[])[][]]>([
    [
      '絞り込んでいなければ、空のグループも出し、「その他」を最後に置く',
      true,
      [
        ['アニメ', ['c']],
        ['ドラマ', ['b', 'd']],
        ['空', []],
        // 消えたグループ (g-gone) のルールは、「その他」に並べる。
        ['その他', ['a', 'e']],
      ],
    ],
    [
      '絞り込んでいれば、ルールのないグループの見出しを出さない',
      false,
      [
        ['アニメ', ['c']],
        ['ドラマ', ['b', 'd']],
        ['その他', ['a', 'e']],
      ],
    ],
  ])('%s', (_, showEmpty, want) => {
    expect(layout(ruleSections(sorted, groups, showEmpty))).toEqual(want);
  });

  test('「その他」は、ルールがなければ出さない', () => {
    expect(layout(ruleSections([grouped('c', 'g-anime')], groups.slice(0, 1), false))).toEqual([
      ['アニメ', ['c']],
    ]);
  });

  test('グループが 1 つもなければ、「その他」の見出し 1 つにまとめる', () => {
    expect(layout(ruleSections([], [], true))).toEqual([['その他', []]]);
    expect(layout(ruleSections([grouped('a', null)], [], false))).toEqual([['その他', ['a']]]);
  });

  test('見出しの鍵は、「その他」とグループで重ならない', () => {
    expect(sectionKey(null)).not.toBe(sectionKey(groups[0]!));
  });
});
