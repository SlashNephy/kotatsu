import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { GraphQLRequestError } from '../../relay/requestError';
import { asViewer, call, merge, minutesAgo } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// タイムシフト (/timeshift/$serviceId) の画面 (#2056)。バッファにある番組 (放送中、頭から録れていない、名前のない、昇格中) と、
// 録画への昇格の結果、昇格できないユーザー、サービスがないとき、読み込めなかったときの表示を並べる。
const meta = {
  title: 'ページ/タイムシフト',
  component: PageStory,
  args: { url: '/timeshift/Service:1024' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// programs は、バッファにある番組を、古い順に並べたもの (画面は新しい順に並べ直す)。
const programs = () => [
  {
    name: '朝のニュース',
    startAt: minutesAgo(240),
    endAt: minutesAgo(180),
    fromStart: false,
    promoting: false,
  },
  { name: '', startAt: minutesAgo(180), endAt: minutesAgo(150), fromStart: true, promoting: false },
  {
    name: '[字]ドキュメンタリー「森の一年」',
    startAt: minutesAgo(150),
    endAt: minutesAgo(60),
    fromStart: true,
    promoting: true,
  },
  { name: '昼のワイド', startAt: minutesAgo(60), endAt: null, fromStart: true, promoting: false },
];

// withPrograms は、サービスのタイムシフトのバッファに、番組を入れる。
const withPrograms = (list: () => object[]) => (root: DemoRoot) => ({
  node: (args: { id: string }) => {
    const node = call<Record<string, unknown> | null>(root, 'node', args);

    return node === null ? null : { ...node, timeshiftPrograms: list };
  },
});

// promoteLast は、いちばん新しい番組 (一覧のいちばん上) の「録画に残す」を押す。
const promoteLast: Story['play'] = async ({ canvas }) => {
  const [first] = await canvas.findAllByRole('button', { name: '録画に残す' });
  if (first !== undefined) {
    await userEvent.click(first);
  }
};

// signedOut は、番組の一覧の読み込みを、ログインしていないときの拒否 (HTTP 401) で失敗させる。
const signedOut = (operation: { name: string }) => {
  if (operation.name === 'ServiceIdTimeshiftQuery') {
    throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
  }

  return false;
};

// 既定。バッファに番組がないとき。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// バッファに番組があるとき。放送中、先頭から録れていない、名前のない、録画として出力している途中の番組を含む。
export const Programs: Story = { args: { overrides: withPrograms(programs) } };

// 視聴者 (予約を作れない) のとき。「録画に残す」を出さない。
export const AsViewer: Story = {
  args: { overrides: merge(withPrograms(programs), asViewer('VIEWER')) },
};

// 放送中の番組を昇格して、番組予約を作ったとき。
export const Reserved: Story = {
  args: {
    overrides: merge(withPrograms(programs), () => ({
      promoteTimeshiftProgram: () => ({ recording: null, reservation: { id: 'Reservation:1' } }),
    })),
  },
  play: promoteLast,
};

// 放送の終わった番組を昇格して、録画として出力し始めたとき。
export const Promoted: Story = {
  args: {
    overrides: merge(withPrograms(programs), () => ({
      promoteTimeshiftProgram: () => ({ recording: { id: 'Recording:1' }, reservation: null }),
    })),
  },
  play: promoteLast,
};

// ほかで同じ番組を昇格していたとき (出力している途中として出す)。
export const PromoteConflict: Story = {
  args: {
    overrides: merge(withPrograms(programs), () => ({
      promoteTimeshiftProgram: failingWith({ code: 'CONFLICT' }),
    })),
  },
  play: promoteLast,
};

// 昇格できなかったとき。
export const PromoteFailed: Story = {
  args: {
    overrides: merge(withPrograms(programs), () => ({ promoteTimeshiftProgram: failing() })),
  },
  play: promoteLast,
};

// サービスが見つからないとき。
export const NotFound: Story = { args: { url: '/timeshift/Service:missing' } };

// ログインしていないとき (ログインへのリンク)。
export const SignedOut: Story = { args: { hold: signedOut } };

// 読み込めなかったとき。node は null を許すので、失敗させてもサービスがないときと同じになる。問い合わせごと失敗させる。
export const Error: Story = {
  args: {
    hold: (operation) => {
      if (operation.name === 'ServiceIdTimeshiftQuery') {
        throw new globalThis.Error('the demo server failed');
      }

      return false;
    },
  },
};
