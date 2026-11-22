import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import type { Executor } from '../../demo/executor';
import { broadcastDay, dateOf } from '../../guide/layout';
import { GraphQLRequestError } from '../../relay/environment';
import { call, merge, withStorage } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// 番組表 (/guide) の画面 (#2056)。日付の移動、種別とピン留めのタブ、番組表ビュー、枠の印 (録画予約、視聴予約、録画、
// サブチャンネル)、まだ届いていない時間帯、端末の表示の設定を、Story ごとに並べる。番組の詳細のパネルは、番組表/番組の詳細にまとめる。
const meta = {
  title: 'ページ/番組表',
  component: PageStory,
  args: { url: '/guide' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Node = Record<string, unknown>;
type GuideRow = { service: Node & { id: string }; programs: Node[] };

const day = 24 * 3_600_000;

// rejecting は、名前が guideQuery の読み込みを、error で失敗させる。hold の中で投げると、Relay に失敗として届く。
const rejecting = (error: () => unknown) => (op: { name: string }) => {
  if (op.name === 'guideQuery') {
    throw error();
  }

  return false;
};

// dayPath は、今日から days 日後の番組表のパスを返す。
const dayPath = (days: number) => () =>
  Promise.resolve(`/guide?date=${dateOf(broadcastDay(Date.now() + days * day).from)}`);

// patchGuide は、番組表の行を patch で変える。
function patchGuide(patch: (rows: GuideRow[], args: { from: string; to: string }) => GuideRow[]) {
  return (root: DemoRoot) => ({
    guide: (args: { from: string; to: string }) =>
      patch(call<GuideRow[]>(root, 'guide', args), args),
  });
}

// withView は、ジャンルで絞り、番組のないサービスを隠す番組表ビューを作る。
function withView(root: DemoRoot) {
  call(root, 'createGuideView', {
    input: { name: 'アニメだけ', genres: [{ lv1: 7 }], hideEmptyServices: true, freeOnly: false },
  });

  return {};
}

// viewPath は、作った番組表ビューを当てた番組表のパスを返す。
async function viewPath(executor: Executor) {
  const result = (await executor.execute('{ guideViews { id } }', {})) as {
    data?: { guideViews?: { id: string }[] };
  };
  const id = result.data?.guideViews?.[0]?.id;
  if (id === undefined) {
    throw new globalThis.Error('the demo has no guide views');
  }

  return `/guide?view=${encodeURIComponent(id)}`;
}

// withMarks は、枠の印を散らす。これからの番組に視聴予約を付け、いくつかの番組に録画があることにし、
// 2 つ目の地上波のサービスの番組の半分を、1 つ目のサービスと同じ番組を流すサブチャンネルの枠にする。
function withMarks(root: DemoRoot) {
  const now = Date.now();
  const rows = call<GuideRow[]>(root, 'guide', {
    from: new Date(now).toISOString(),
    to: new Date(now + 12 * 3_600_000).toISOString(),
  });
  for (const row of rows.slice(0, 3)) {
    const next = row.programs.find((program) => Date.parse(String(program['startAt'])) > now);
    if (next !== undefined) {
      call(root, 'setWatchReservation', { program: next['id'], reserved: true });
    }
  }

  return patchGuide((guide) => {
    const main = guide[0]?.service;

    return guide.map((row, rowIndex) => ({
      ...row,
      programs: row.programs.map((program, index) => ({
        ...program,
        recorded: index % 5 === 0,
        sharedFrom:
          rowIndex === 1 && index % 2 === 0 && main !== undefined ? { id: main.id } : null,
      })),
    }));
  })(root);
}

export const Default: Story = {};

// 読み込み中。tenban が応答を返すまでの画面。
export const Loading: Story = { args: { hold: holdAll } };

// 番組表を読めなかったとき。
export const Error: Story = { args: { overrides: { guide: failing() } } };

// ログインが切れていて、番組表を読めなかったとき。
export const SignedOut: Story = {
  args: {
    hold: rejecting(() => new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED')),
  },
};

// 消した番組表ビューのリンクを開いたとき。ビューを外して開き直すリンクを出す。
export const ViewNotFound: Story = {
  args: {
    url: '/guide?view=GuideView:missing',
    hold: rejecting(() => new GraphQLRequestError('not found', 404, 'NOT_FOUND')),
  },
};

// 番組表に番組がないとき (EPG をまだ受け取っていないなど)。ピン留めのタブだけを出す。
export const Empty: Story = { args: { overrides: { guide: () => [] } } };

// 番組表に番組がなく、ピン留めもないとき。種別のタブを出さない。
export const EmptyWithoutPins: Story = {
  args: { overrides: { guide: () => [], pinnedServices: () => [] } },
};

// ピン留めのタブ (#485)。ピン留めした順にサービスを並べる。
export const PinnedTab: Story = { args: { url: '/guide?type=pinned' } };

// ピン留めがないのに、ピン留めのタブの URL を開いたとき。最初の種別を出す。
export const PinnedTabWithoutPins: Story = {
  args: { url: '/guide?type=pinned', overrides: { pinnedServices: () => [] } },
};

// BS のタブ。
export const BsTab: Story = { args: { url: '/guide?type=BS' } };

// 暦にない日付の URL を開いたとき (#1430)。今日の番組表を出す。
export const InvalidDate: Story = { args: { url: '/guide?date=2026-02-30' } };

// 明日の番組表。前の日へ戻れ、いまの時刻の線は引かない。
export const Tomorrow: Story = { args: { url: dayPath(1) } };

// 移れる最後の日の番組表。次の日へは移れない。
export const LastDay: Story = { args: { url: dayPath(7) } };

// 番組表が途中までしか届いていない日。届いていない時間帯を斜線で塗り、案内を出す。
export const NotReceived: Story = {
  args: {
    url: dayPath(7),
    overrides: patchGuide((rows, args) =>
      rows.map((row) => ({
        ...row,
        programs: row.programs.filter(
          (program) =>
            Date.parse(String(program['startAt'])) < Date.parse(args.to) - 10 * 3_600_000,
        ),
      })),
    ),
  },
};

// 過去の番組を残しているとき (#1121)。今日より前の日へ戻れ、日付のメニューで過去の日を選べる。
export const PastDays: Story = {
  args: {
    overrides: { oldestArchivedProgramAt: new Date(Date.now() - 3 * day).toISOString() },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: /^日付を選ぶ/ }));
  },
};

// 時間帯のメニュー。深夜を 24 時以降で表す切り替えもここにある。
export const JumpMenu: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '時間帯' }));
  },
};

// 深夜を 24 時以降で表すとき (#486)。端末に覚えた設定を読む。
export const ExtendedHours: Story = {
  beforeEach: () => withStorage({ 'kotatsu.guide.extendedHours': 'true' }),
};

// 番組表の密度を「狭い」にしたとき。端末に覚えた見た目の設定を読む。
export const CompactDensity: Story = {
  beforeEach: () =>
    withStorage({ 'kotatsu.guide.appearance': JSON.stringify({ density: 'compact' }) }),
};

// 枠の印。録画予約 (ルール予約と番組予約)、視聴予約、録画があることと、サブチャンネルで同じ番組を流す枠を出す。
export const Marks: Story = { args: { overrides: withMarks } };

// その日ずっと 1 つ目のサービスと同じ番組を流しているサブチャンネルは、列を出さない。
export const SubchannelCollapsed: Story = {
  args: {
    overrides: patchGuide((rows) => {
      const main = rows[0]?.service;

      return rows.map((row, index) =>
        index === 1 && main !== undefined
          ? {
              ...row,
              programs: row.programs.map((program) => ({
                ...program,
                sharedFrom: { id: main.id },
              })),
            }
          : row,
      );
    }),
  },
};

// 番組表ビューを当てたとき。ビューの条件で絞り、編集のボタンを出す。
export const WithView: Story = { args: { url: viewPath, overrides: withView } };

// 番組表ビューを切り替えて読み込んでいるとき。前の番組表の上に、読み込み中を重ねる。
export const SwitchingView: Story = {
  args: {
    overrides: withView,
    hold: (operation, variables) => operation.name === 'guideQuery' && variables['view'] != null,
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByLabelText('番組表ビュー'));
    await userEvent.click(await within(document.body).findByRole('option', { name: 'アニメだけ' }));
  },
};

// 番組表ビューを作るダイアログ。
export const CreateView: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'ビューを作成' }));
  },
};

// 番組表ビューを保存できなかったとき。
export const CreateViewError: Story = {
  args: { overrides: { createGuideView: failing() } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'ビューを作成' }));
    const dialog = within(await within(document.body).findByRole('dialog'));
    await userEvent.type(dialog.getByRole('textbox', { name: 'タイトル' }), '夜のドラマ');
    await userEvent.click(dialog.getByRole('button', { name: '保存' }));
  },
};

// 番組表ビューを編集するダイアログ。削除のボタンも出す。
export const EditView: Story = {
  args: { url: viewPath, overrides: withView },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'ビューを編集' }));
  },
};

// 番組表ビューを消せなかったとき。
export const DeleteViewError: Story = {
  args: {
    url: viewPath,
    overrides: merge(withView, () => ({ deleteGuideView: failing() })),
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'ビューを編集' }));
    const dialog = within(await within(document.body).findByRole('dialog'));
    await userEvent.click(dialog.getByRole('button', { name: '削除' }));
  },
};

// 予約の一覧やホームから、予約の番組を開いたとき。番組表の中から予約の番組を探し、その詳細を開く。
export const FromReservation: Story = {
  args: {
    url: async (executor: Executor) => {
      const now = Date.now();
      const result = (await executor.execute(
        'query ($from: Time!, $to: Time!) { reservations(from: $from, to: $to, first: 1) { edges { node { id startAt service { channelType { type } } } } } }',
        { from: new Date(now).toISOString(), to: new Date(now + day).toISOString() },
      )) as {
        data?: {
          reservations?: {
            edges: {
              node: { id: string; startAt: string; service: { channelType: { type: string } } };
            }[];
          };
        };
      };
      const node = result.data?.reservations?.edges[0]?.node;
      if (node === undefined) {
        throw new globalThis.Error('the demo has no reservations');
      }
      const search = new URLSearchParams({
        date: dateOf(broadcastDay(Date.parse(node.startAt)).from),
        type: node.service.channelType.type,
        reservation: node.id,
      });

      return `/guide?${search.toString()}`;
    },
  },
};

// 予約の番組が番組表に見つからないとき。予約を外すだけで、詳細は開かない。
export const FromMissingReservation: Story = {
  args: { url: () => Promise.resolve('/guide?reservation=Reservation:missing') },
};

// 放送がロゴを送っていないサービス。列の見出しに、ロゴの代わりにサービス名を出す。
export const NoLogos: Story = {
  args: {
    overrides: patchGuide((rows) =>
      rows.map((row) => ({ ...row, service: { ...row.service, logoUrl: null } })),
    ),
  },
};
