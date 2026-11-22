import type { Meta, StoryObj } from '@storybook/react-vite';

import type { DemoRoot } from '../../demo';
import type { Executor } from '../../demo/executor';
import { GraphQLRequestError } from '../../relay/environment';
import { call, withStorage } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// サービスごとの番組表 (/guide/channel/$serviceId) の画面 (#2056)。1 つのチャンネルの 8 日分を、日付を列にして並べる。
const meta = {
  title: 'ページ/番組表/サービスごと',
  component: PageStory,
  args: { url: '/guide/channel/Service:1024' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type GuideRow = { service: object; programs: Record<string, unknown>[] };

const day = 24 * 3_600_000;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

export const Error: Story = { args: { overrides: { guide: failing() } } };

// ログインが切れていて、番組表を読めなかったとき。hold の中で投げると、Relay に失敗として届く。
export const SignedOut: Story = {
  args: {
    hold: (operation) => {
      if (operation.name === 'channelServiceIdGuideQuery') {
        throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
      }

      return false;
    },
  },
};

// サービスが見つからないとき (上流から消えたサービスの URL を開いたなど)。
export const NotFound: Story = { args: { url: '/guide/channel/Service:9999' } };

// サービスはあるが、8 日分の番組がないとき。
export const Empty: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      guide: (args: object) =>
        call<GuideRow[]>(root, 'guide', args).map((row) => ({ ...row, programs: [] })),
    }),
  },
};

// 後ろの日の番組表が、まだ届いていないとき。日ごとの列に、届いていない時間帯を塗る。
export const NotReceived: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      guide: (args: object) =>
        call<GuideRow[]>(root, 'guide', args).map((row) => ({
          ...row,
          programs: row.programs.filter(
            (program) => Date.parse(String(program['startAt'])) < Date.now() + 5 * day,
          ),
        })),
    }),
  },
};

// 深夜を 24 時以降で表すとき (#486)。番組表の画面と同じ、端末に覚えた設定を読む。
export const ExtendedHours: Story = {
  beforeEach: () => withStorage({ 'kotatsu.guide.extendedHours': 'true' }),
};

// 番組の詳細を開いたとき。これからの番組を 1 つ選んで開く。
export const ProgramDetail: Story = {
  args: {
    url: async (executor: Executor) => {
      const now = Date.now();
      const result = (await executor.execute(
        'query ($from: Time!, $to: Time!) { guide(from: $from, to: $to, services: ["Service:1024"]) { programs { id startAt } } }',
        { from: new Date(now).toISOString(), to: new Date(now + day).toISOString() },
      )) as { data?: { guide?: { programs: { id: string; startAt: string }[] }[] } };
      const program = result.data?.guide?.[0]?.programs.find(
        (candidate) => Date.parse(candidate.startAt) > now,
      );
      if (program === undefined) {
        throw new globalThis.Error('the demo has no programs');
      }

      return `/guide/channel/Service:1024?program=${encodeURIComponent(program.id)}`;
    },
  },
};
