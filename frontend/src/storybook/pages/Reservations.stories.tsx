import type { Meta, StoryObj } from '@storybook/react-vite';
import { fireEvent, userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { paginate } from '../../demo/data';
import { GraphQLRequestError } from '../../relay/environment';
import { asViewer, call, merge } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// 予約の一覧 (/reservations) の画面 (#2056)。録画予約、視聴予約、重複で見送った番組、見たいリストのタブと、
// 予約の行の状態 (放送中、競合、一部のみ、無効)、まとめての操作、行のメニューとダイアログを、Story ごとに並べる。
const meta = {
  title: 'ページ/予約',
  component: PageStory,
  args: { url: '/reservations' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Node = Record<string, unknown>;
type Connection = { edges: { node: Node; cursor: string }[]; pageInfo: Record<string, unknown> };
type GuideRow = { service: Node; programs: (Node & { startAt: string })[] };

// rejecting は、名前が operation の読み込みを、error で失敗させる。hold の中で投げると、Relay に失敗として届く。
// GraphQL の応答のエラーでは作れない失敗 (ログインが切れたなど) を作るのに使う。
const rejecting =
  (operation: string, error: () => unknown = () => new globalThis.Error('rejected')) =>
  (op: { name: string }) => {
    if (op.name === operation) {
      throw error();
    }

    return false;
  };

// signedOut は、operation の読み込みを、ログインしていないために拒まれたことにする。
const signedOut = (operation: string) =>
  rejecting(operation, () => new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED'));

// upcomingPrograms は、デモの番組表のうち、これから始まる番組を、始まりの早い順に返す。
function upcomingPrograms(root: DemoRoot) {
  const now = Date.now();
  const rows = call<GuideRow[]>(root, 'guide', {
    from: new Date(now).toISOString(),
    to: new Date(now + 24 * 3_600_000).toISOString(),
  });

  return rows
    .flatMap((row) => row.programs)
    .filter((program) => Date.parse(program.startAt) > now)
    .toSorted((a, b) => Date.parse(a.startAt) - Date.parse(b.startAt));
}

// patchReservations は、予約の一覧の行を、始まりの早い順に patch で変える。状態で絞る問い合わせ (states) には、
// 変えた後の状態で絞って返す。
function patchReservations(patch: (node: Node, index: number, root: DemoRoot) => Node) {
  return (root: DemoRoot) => ({
    reservations: (args: { states?: string[] | null; first: number } & Record<string, unknown>) => {
      const { states, ...rest } = args;
      const all = call<Connection>(root, 'reservations', { ...rest, first: 1000 });
      const nodes = all.edges
        .map((edge, index) => patch(edge.node, index, root))
        .filter(
          (node) =>
            states == null || states.includes((node['assignment'] as { state: string }).state),
        );

      return paginate(nodes, args.first, null);
    },
  });
}

// peersOf は、チューナーを取り合っている相手の予約 (#1502) を、架空の番組名で作る。
function peersOf(root: DemoRoot, names: string[]) {
  const services = call<Node[]>(root, 'services');

  return names.map((name, index) => ({
    __typename: 'Reservation',
    id: `Reservation:peer-${index}`,
    name,
    service: services[index % services.length],
  }));
}

// withAttention は、予約の先頭の 2 件を、競合と一部のみにする。どちらも取り合っている相手を添える。
const withAttention = patchReservations((node, index, root) => {
  if (index > 1) {
    return node;
  }
  const assignment = node['assignment'] as Node;
  const conflict = index === 0;

  return {
    ...node,
    enabled: true,
    assignment: {
      ...assignment,
      state: conflict ? 'CONFLICT' : 'PARTIAL',
      tuner: conflict ? null : 0,
      plannedStartAt: conflict
        ? null
        : new Date(Date.parse(String(node['startAt'])) + 5 * 60_000).toISOString(),
      plannedEndAt: conflict ? null : node['endAt'],
      // 名前のない相手 (名前なしの時刻予約) も混ぜる。
      peers: peersOf(root, conflict ? ['[新]縁側の午後', ''] : ['こたつ探偵 #3']),
    },
  };
});

// withOnAir は、放送中の時刻予約 (名前なし) を足す。いま録画中で、一覧の先頭の「放送中」の見出しの下に出る。
function withOnAir(root: DemoRoot) {
  const now = Date.now();
  call(root, 'createTimeReservation', {
    input: {
      service: 'Service:1024',
      startAt: new Date(now - 10 * 60_000).toISOString(),
      endAt: new Date(now + 50 * 60_000).toISOString(),
      name: '',
    },
  });

  return {};
}

// rowOf は、種類 (label) のマークの付いた、最初の予約の行を返す。
async function rowOf(canvasElement: HTMLElement, label: string) {
  const [icon] = await within(canvasElement).findAllByRole('img', { name: label });
  const row = icon?.parentElement?.parentElement;
  if (row == null) {
    throw new globalThis.Error(`no row of ${label}`);
  }

  return within(row);
}

// openMenu は、種類 (label) の最初の行の、操作のメニューを開く。
async function openMenu(canvasElement: HTMLElement, label: string) {
  const row = await rowOf(canvasElement, label);
  await userEvent.click(row.getByRole('button', { name: '予約の操作' }));
}

// selectFirst は、選ぶのを始め、選べる最初の予約を選ぶ。
async function selectFirst(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await userEvent.click(await canvas.findByRole('button', { name: '選ぶ' }));
  const boxes = await canvas.findAllByRole('checkbox');
  const box = boxes.find((candidate) => !(candidate as HTMLInputElement).disabled);
  await userEvent.click(box!);
}

const body = within(document.body);

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 予約が 1 つもないとき。
export const Empty: Story = {
  args: {
    overrides: {
      reservations: (args: { first: number; after?: string | null }) =>
        paginate([], args.first, args.after),
    },
  },
};

export const Error: Story = { args: { overrides: { reservations: failing() } } };

// ログインが切れていて、一覧を読めなかったとき。ログインの画面へのリンクを出す。
export const SignedOut: Story = { args: { hold: signedOut('reservationsQuery') } };

// 競合と一部のみの予約があるとき。行のバッジ、録れる予定の時刻、取り合っている相手、上の件数のバッジを出す。
export const Attention: Story = { args: { overrides: withAttention } };

// 「録画が欠ける見込み」で絞ったとき。件数のバッジは出さない。
export const ProblemsOnly: Story = {
  args: { url: '/reservations?problems=true', overrides: withAttention },
};

// 「録画が欠ける見込み」で絞ったが、欠ける予約がないとき。
export const NoProblems: Story = {
  args: {
    url: '/reservations?problems=true',
    overrides: patchReservations((node) => ({
      ...node,
      assignment: { ...(node['assignment'] as Node), state: 'OK' },
    })),
  },
};

// ルールに一致した予約だけを出しているとき (#468)。ルールの名前と、絞り込みを外すボタンを出す。
export const FilteredByRule: Story = { args: { url: '/reservations?rule=Rule:1' } };

// 絞り込んだルールが見つからないとき (消したルールのリンクを開いたなど)。
export const FilteredByMissingRule: Story = { args: { url: '/reservations?rule=Rule:999' } };

// 放送中の予約があるとき。見出しの前にまとめ、録画中のバッジを出す。名前のない時刻予約は「名前なし」と出す。
export const OnAir: Story = { args: { overrides: withOnAir } };

// 同じ回を録画済みと分かって Kotatsu が無効にした予約で、先に録った局が分からないとき (#1500)。
export const RecordedEpisodeUnknownService: Story = {
  args: {
    overrides: patchReservations((node) =>
      node['disabledReason'] === 'RECORDED_EPISODE' ? { ...node, disabledService: null } : node,
    ),
  },
};

// 視聴者から見たとき。選ぶボタン、時刻予約のボタン、行のメニューを出さない。
export const AsViewer: Story = { args: { overrides: asViewer('VIEWER') } };

// まとめて操作する予約を選んでいるとき。録画中の予約は選べない。
export const Selecting: Story = {
  args: { overrides: withOnAir },
  play: async ({ canvasElement }) => selectFirst(canvasElement),
};

// まとめて有効にした結果。録画中などで飛ばした予約の件数も出す。
export const BulkResult: Story = {
  args: { overrides: { setReservationsEnabled: () => ({ processed: 1, skipped: 1 }) } },
  play: async ({ canvas, canvasElement }) => {
    await selectFirst(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: '選んだ1件を有効にする' }));
  },
};

// まとめての操作に失敗したとき。
export const BulkError: Story = {
  args: { overrides: { setReservationsEnabled: failing() } },
  play: async ({ canvas, canvasElement }) => {
    await selectFirst(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: '選んだ1件を無効にする' }));
  },
};

// 選んだ予約をまとめて消す前の確認。
export const BulkDeleteConfirm: Story = {
  play: async ({ canvas, canvasElement }) => {
    await selectFirst(canvasElement);
    await userEvent.click(canvas.getByRole('button', { name: '選んだ1件を削除' }));
  },
};

// 時刻予約の行のメニュー。時刻と名前を変える項目と、削除を出す。
export const TimeMenu: Story = {
  args: { overrides: withOnAir },
  play: async ({ canvasElement }) => openMenu(canvasElement, '時刻予約'),
};

// ルール予約の行のメニュー。消しても次の照合で作られるので、削除を出さない。
export const RuleMenu: Story = {
  play: async ({ canvasElement }) => openMenu(canvasElement, 'ルール予約'),
};

// 有効と無効を切り替えられなかったとき。行の下に通知する。
export const ToggleError: Story = {
  args: { overrides: { updateReservation: failing() } },
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, 'ルール予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '無効にする' }));
  },
};

// 番組予約を消す前の確認。
export const DeleteDialog: Story = {
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, '番組予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '削除' }));
  },
};

// 番組予約を消せなかったとき。
export const DeleteError: Story = {
  args: { overrides: { deleteReservation: failing() } },
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, '番組予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '削除' }));
    const dialog = within(await body.findByRole('dialog'));
    await userEvent.click(dialog.getByRole('button', { name: '削除' }));
  },
};

// ルール予約の録画の設定。ルールの設定を引き継ぐことを示す。
export const RuleSettingsDialog: Story = {
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, 'ルール予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '録画の設定' }));
  },
};

// 番組予約の録画の設定。
export const ProgramSettingsDialog: Story = {
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, '番組予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '録画の設定' }));
  },
};

// 録画の設定を読み込めなかったとき (予約が消えていたなど)。
export const SettingsLoadError: Story = {
  args: { overrides: { node: () => null } },
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, '番組予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '録画の設定' }));
  },
};

// 時刻予約を作るダイアログ。
export const CreateTimeReservation: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '時刻予約を作成' }));
  },
};

// 時刻予約の終わりを、始まりより前にしたとき。
export const TimeReservationOrderError: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '時刻予約を作成' }));
    const end = await body.findByLabelText('終了', { exact: false });
    await fireEvent.change(end, { target: { value: '2000-01-01T00:00' } });
  },
};

// 時刻予約の長さを、24 時間より長くしたとき。
export const TimeReservationTooLong: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '時刻予約を作成' }));
    const end = await body.findByLabelText('終了', { exact: false });
    await fireEvent.change(end, { target: { value: '2099-01-01T00:00' } });
  },
};

// 時刻予約の時刻と名前を変えられなかったとき。
export const TimeReservationSaveError: Story = {
  args: { overrides: merge(withOnAir, () => ({ updateReservation: failing() })) },
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, '時刻予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '時刻と名前を変える' }));
    const dialog = within(await body.findByRole('dialog'));
    await userEvent.click(dialog.getByRole('button', { name: '保存' }));
  },
};

// 時刻予約を作るダイアログで、サービスの一覧を読み込めなかったとき。
export const TimeReservationLoadError: Story = {
  args: { hold: rejecting('TimeReservationDialogQuery') },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '時刻予約を作成' }));
  },
};

// smallPages は、1 回に取る件数を 3 件にして、続きがあることにする。
const smallPages = (root: DemoRoot) => ({
  reservations: (args: { first: number } & Record<string, unknown>) =>
    call<Connection>(root, 'reservations', { ...args, first: Math.min(args.first, 3) }),
});

// 1 回に取る件数より予約が多いとき。続きを読み込むボタンを出す。
export const HasMore: Story = { args: { overrides: smallPages } };

// 続きを読み込めなかったとき。
export const MoreError: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      reservations: (args: { first: number; after?: string | null } & Record<string, unknown>) => {
        if (args.after != null) {
          throw new globalThis.Error('the demo server failed');
        }

        return smallPages(root).reservations(args);
      },
    }),
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'さらに読み込む' }));
  },
};

// 視聴予約のタブ。視聴予約がないとき。
export const WatchEmpty: Story = { args: { url: '/reservations?watch=true' } };

// withWatchReservations は、これからの番組の 3 つに、視聴予約を付ける。
function withWatchReservations(root: DemoRoot) {
  for (const program of upcomingPrograms(root).slice(0, 3)) {
    call(root, 'setWatchReservation', { program: program['id'], reserved: true });
  }

  return {};
}

// 視聴予約のタブ。視聴予約が並ぶとき。
export const WatchReservations: Story = {
  args: { url: '/reservations?watch=true', overrides: withWatchReservations },
};

// 視聴予約を外せなかったとき。ベルのボタンに通知する。
export const WatchReservationError: Story = {
  args: {
    url: '/reservations?watch=true',
    overrides: merge(withWatchReservations, () => ({ setWatchReservation: failing() })),
  },
  play: async ({ canvas }) => {
    const [button] = await canvas.findAllByRole('button', { name: '視聴予約を外す' });
    await userEvent.click(button!);
  },
};

// 重複で見送った番組のタブ。見送った番組がないとき。
export const SkippedEmpty: Story = { args: { url: '/reservations?skipped=true' } };

// withSkipped は、見送った理由ごとの番組を並べる (#469、#1498、#1499)。
function withSkipped(root: DemoRoot) {
  const programs = upcomingPrograms(root);
  const services = call<Node[]>(root, 'services');
  const rules = [{ id: 'Rule:1', name: 'こたつ探偵' }];
  const at = (index: number) => programs[index % programs.length]!;
  const earlier = new Date(Date.now() - 3 * 24 * 3_600_000).toISOString();
  const entries = [
    // 同じ番組名を期間内に録画済み。
    { reason: 'DUPLICATE_TITLE', lastRecordedAt: earlier },
    // 同じ番組名を録画済みだが、録画の日が分からない。
    { reason: 'DUPLICATE_TITLE', lastRecordedAt: null },
    // 同じ回を、ほかの局で録画済み。
    { reason: 'RECORDED_EPISODE', recorded: { service: services[7], name: '', endAt: earlier } },
    // 同じ回を録画済みだが、録った局が分からない。
    { reason: 'RECORDED_EPISODE', recorded: { service: null, name: '', endAt: earlier } },
    // 同じ回を、別の放送で録る。
    { reason: 'OTHER_BROADCAST', instead: at(10) },
    // 同じ回を別の放送で録るが、その放送が分からない。
    { reason: 'OTHER_BROADCAST', instead: null },
    // 追っているシリーズの、前の回。
    { reason: 'EARLIER_EPISODE', laterEpisode: 5 },
  ];

  return {
    skippedDuplicates: () =>
      entries.map((entry, index) => ({
        program: at(index),
        rules,
        lastRecordedAt: null,
        recorded: null,
        instead: null,
        laterEpisode: null,
        ...entry,
      })),
  };
}

// 重複で見送った番組のタブ。理由ごとの説明を出す。
export const Skipped: Story = {
  args: { url: '/reservations?skipped=true', overrides: withSkipped },
};

// 見送った番組を予約できなかったとき。
export const SkippedReserveError: Story = {
  args: {
    url: '/reservations?skipped=true',
    overrides: merge(withSkipped, () => ({ createProgramReservation: failing() })),
  },
  play: async ({ canvas }) => {
    const [button] = await canvas.findAllByRole('button', { name: '予約する' });
    await userEvent.click(button!);
  },
};

// wishlistEnabled は、管理者がアニメの結びつきを有効にしていることにする。見たいリストのタブが出る。
function wishlistEnabled(root: DemoRoot) {
  return {
    animeRecordSettings: () => ({ ...call<Node>(root, 'animeRecordSettings'), enabled: true }),
  };
}

// wishlistWith は、見たいリストの候補を wishlist にする。
function wishlistWith(wishlist: (root: DemoRoot) => object) {
  return merge(wishlistEnabled, (root) => ({ animeWishlist: () => wishlist(root) }));
}

// fullWishlist は、取り込みの状態と、作品の候補を並べた見たいリスト。
const fullWishlist = (root: DemoRoot) => {
  const programs = upcomingPrograms(root);
  const imported = new Date(Date.now() - 2 * 3_600_000).toISOString();

  return {
    enabled: true,
    imports: [
      // 取り込めた記録先。
      { destination: 'ANNICT', importedAt: imported, error: null, entryCount: 12 },
      // 取り込みに失敗した記録先。
      { destination: 'ANILIST', importedAt: null, error: 'the token was revoked', entryCount: 0 },
    ],
    works: [
      {
        tid: 1,
        title: 'こたつ探偵',
        reservedBroadcasts: 0,
        covered: false,
        sources: [
          {
            destination: 'ANNICT',
            workId: 101,
            status: 'WATCHING',
            url: 'https://example.com/works/101',
          },
        ],
        broadcasts: [
          { preferred: true, program: programs[0] },
          { preferred: false, program: programs[1] },
        ],
      },
      {
        tid: 2,
        title: '縁側の午後',
        reservedBroadcasts: 2,
        covered: true,
        sources: [
          {
            destination: 'ANILIST',
            workId: 202,
            status: 'WANNA_WATCH',
            url: 'https://example.com/anime/202',
          },
        ],
        broadcasts: [{ preferred: false, program: programs[2] }],
      },
    ],
  };
};

// 見たいリストのタブ。管理者が見たいリストを無効にしているとき。
export const WishlistDisabled: Story = {
  args: { url: '/reservations?wishlist=true', overrides: wishlistEnabled },
};

// 見たいリストのタブ。記録先とつないでいないとき。
export const WishlistNotConnected: Story = {
  args: {
    url: '/reservations?wishlist=true',
    overrides: wishlistWith(() => ({ enabled: true, imports: [], works: [] })),
  },
};

// 見たいリストのタブ。取り込みを待っていて、候補がまだないとき。
export const WishlistEmpty: Story = {
  args: {
    url: '/reservations?wishlist=true',
    overrides: wishlistWith(() => ({
      enabled: true,
      imports: [{ destination: 'ANNICT', importedAt: null, error: null, entryCount: 0 }],
      works: [],
    })),
  },
};

// 見たいリストのタブ。取り込みの結果と失敗、予約済みの作品、おすすめの放送を出す。
export const Wishlist: Story = {
  args: { url: '/reservations?wishlist=true', overrides: wishlistWith(fullWishlist) },
};

// 見たいリストのタブを、視聴者から見たとき。ルールも予約も作れないと知らせ、ボタンを出さない。
export const WishlistAsViewer: Story = {
  args: {
    url: '/reservations?wishlist=true',
    overrides: merge(wishlistWith(fullWishlist), asViewer('VIEWER')),
  },
};

// 見たいリストの作品から、ルールを作ったとき。作ったルールへのリンクを出す。
export const WishlistRuleCreated: Story = {
  args: {
    url: '/reservations?wishlist=true',
    overrides: merge(wishlistWith(fullWishlist), () => ({
      createAnimeWishlistRule: () => ({ __typename: 'Rule', id: 'Rule:1', name: 'こたつ探偵' }),
    })),
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'ルールを作る' }));
  },
};

// 見たいリストの作品から、ルールを作れなかったとき (デモはルールを作れない)。
export const WishlistRuleError: Story = {
  args: { url: '/reservations?wishlist=true', overrides: wishlistWith(fullWishlist) },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'ルールを作る' }));
  },
};

// 見たいリストの放送を、予約できなかったとき。
export const WishlistReserveError: Story = {
  args: {
    url: '/reservations?wishlist=true',
    overrides: merge(wishlistWith(fullWishlist), () => ({ createProgramReservation: failing() })),
  },
  play: async ({ canvas }) => {
    const [button] = await canvas.findAllByRole('button', { name: '予約する' });
    await userEvent.click(button!);
  },
};

// 見たいリストの取り込み直しを頼んだとき。
export const WishlistRefreshStarted: Story = {
  args: {
    url: '/reservations?wishlist=true',
    overrides: merge(wishlistWith(fullWishlist), () => ({ refreshAnimeWishlist: () => true })),
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '取り込み直す' }));
  },
};

// 見たいリストの取り込み直しを頼めなかったとき (デモは取り込めない)。
export const WishlistRefreshError: Story = {
  args: { url: '/reservations?wishlist=true', overrides: wishlistWith(fullWishlist) },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '取り込み直す' }));
  },
};

// withRuleSettings は、「こたつ探偵」(Rule:1) に録画の設定を入れる。ルール予約の録画の設定に、ルールから引き継ぐ値を出す。
function withRuleSettings(root: DemoRoot) {
  call(root, 'updateRule', {
    id: 'Rule:1',
    input: {
      name: 'こたつ探偵',
      memo: '深夜のアニメを毎話録る',
      conditions: { keywords: [{ text: 'こたつ探偵', fields: ['NAME'] }] },
      avoidDuplicates: true,
      duplicateWindowDays: 7,
      settings: {
        poolId: 'default',
        fileNameTemplate: '{{.Name}}.m2ts',
        startMarginSeconds: 30,
        endMarginSeconds: 60,
        priority: 5,
        joinSegments: true,
        detectCm: true,
        generateCaptions: false,
        transcodePreset: '720p',
      },
    },
  });

  return {};
}

// ルールに録画の設定があるときの、ルール予約の録画の設定。ルールの名前と、引き継ぐ値を出す。
export const RuleSettingsInherited: Story = {
  args: { overrides: withRuleSettings },
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, 'ルール予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '録画の設定' }));
  },
};

// 録画の設定を保存できなかったとき。
export const SettingsSaveError: Story = {
  args: { overrides: { updateReservation: failing() } },
  play: async ({ canvasElement }) => {
    await openMenu(canvasElement, '番組予約');
    await userEvent.click(await body.findByRole('menuitem', { name: '録画の設定' }));
    const dialog = within(await body.findByRole('dialog'));
    await userEvent.click(await dialog.findByRole('button', { name: '保存' }));
  },
};
