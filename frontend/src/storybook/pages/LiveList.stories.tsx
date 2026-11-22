import type { Meta, StoryObj } from '@storybook/react-vite';

import type { DemoRoot } from '../../demo';
import { GraphQLRequestError } from '../../relay/requestError';
import { call } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// ライブ視聴のサービスの一覧 (/live) の画面 (#2056)。ピン留め、人気の実況、チャンネル種別のタブと、放送中の番組がない局、
// 局がないときや読み込めなかったときの表示を並べる。
const meta = {
  title: 'ページ/ライブ',
  component: PageStory,
  args: { url: '/live' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Row = {
  service: { id: string; channelType: { type: string }; jikkyo?: object | null };
  programs: object[];
};

// signedOut は、ライブの一覧の読み込みを、ログインしていないときの拒否 (HTTP 401) で失敗させる。
const signedOut = (operation: { name: string }) => {
  if (operation.name === 'liveServicesQuery') {
    throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
  }

  return false;
};

// withGuide は、デモの番組表の行を change で変える。
const withGuide = (change: (rows: Row[]) => Row[]) => (root: DemoRoot) => ({
  guide: (args: object) => change(call<Row[]>(root, 'guide', args)),
});

// withoutJikkyo は、どの局にも実況のチャンネルがないことにする (人気の実況のタブを出さない)。
const withoutJikkyo = (rows: Row[]) =>
  rows.map((row) => ({ ...row, service: { ...row.service, jikkyo: null } }));

// 既定。ピン留めの局があるので、ピン留めのタブを選んで開く。人気の実況のタブと、実況の勢いも出す。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// ログインしていないとき (ログインへのリンク)。
export const SignedOut: Story = { args: { hold: signedOut } };

// 読み込めなかったとき。
export const Error: Story = { args: { overrides: { guide: failing() } } };

// 局が 1 つもないとき (チューナーサーバーにつないだばかり)。タブも出さない。
export const Empty: Story = { args: { overrides: { guide: () => [], pinnedServices: () => [] } } };

// ピン留めの局がなく、実況もないとき。最初のチャンネル種別 (地上波) のタブを選んで開く。
export const NoPinned: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      pinnedServices: () => [],
      ...withGuide(withoutJikkyo)(root),
    }),
  },
};

// 選べるタブが 1 つだけのとき (地上波の局だけで、ピン留めも実況もない)。タブを出さない。
export const SingleTab: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      pinnedServices: () => [],
      ...withGuide((rows) =>
        withoutJikkyo(rows.filter((row) => row.service.channelType.type === 'GR')),
      )(root),
    }),
  },
};

// 放送中の番組がない局があるとき (放送休止の時間帯など)。番組名の代わりに「番組なし」を出し、時刻と勢いを出さない。
export const NoProgram: Story = {
  args: {
    overrides: withGuide((rows) =>
      rows.map((row, index) => (index % 2 === 0 ? { ...row, programs: [] } : row)),
    ),
  },
};
