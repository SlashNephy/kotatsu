import type { Meta, StoryObj } from '@storybook/react-vite';

import type { DemoRoot } from '../../demo';
import { paginate } from '../../demo/data';
import { asViewer, call, merge, reservationsWith } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// ホーム (/) の画面 (#2056)。チューナーサーバーの不調、時計のずれ、予約の競合の警告、続きの放送、空の欄など、
// ホームが出し分ける表示を、Story ごとに並べる。
const meta = {
  title: 'ページ/ホーム',
  component: PageStory,
  args: { url: '/' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const healthy = {
  degraded: false,
  degradedSince: null,
  clockSkewed: false,
  clockSkewMs: null,
  broadcastClockSkewed: false,
  broadcastClockSkewMs: null,
};

const emptyConnection = (args: { first: number; after?: string | null }) =>
  paginate([], args.first, args.after);

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// チューナーサーバーに 5 分前からつながらないとき。
export const TunerServerDegraded: Story = {
  args: {
    overrides: {
      tunerServerHealth: () => ({
        ...healthy,
        degraded: true,
        degradedSince: new Date(Date.now() - 5 * 60_000).toISOString(),
      }),
    },
  },
};

// サーバーの時計が、チューナーサーバーより 15 秒ずれているとき。
export const ClockSkewed: Story = {
  args: {
    overrides: {
      tunerServerHealth: () => ({ ...healthy, clockSkewed: true, clockSkewMs: 15_000 }),
    },
  },
};

// 放送の時刻 (TOT) が、サーバーの時計より 12 秒遅れているとき。
export const BroadcastClockSkewed: Story = {
  args: {
    overrides: {
      tunerServerHealth: () => ({
        ...healthy,
        broadcastClockSkewed: true,
        broadcastClockSkewMs: -12_000,
      }),
    },
  },
};

// これからの予約の 2 件が、チューナーの競合で録れないとき。
export const Conflicts: Story = { args: { overrides: reservationsWith('CONFLICT', 2) } };

// これからの予約の 1 件が、途中までしか録れないとき。
export const Partial: Story = { args: { overrides: reservationsWith('PARTIAL', 1) } };

// 競合が 100 件を超えるとき。
export const ManyConflicts: Story = { args: { overrides: reservationsWith('CONFLICT', 3, true) } };

// withNextEpisodes は、見たシリーズの続きの放送を出す。デモの番組表のこれからの番組を、続きの放送として並べる。
function withNextEpisodes(root: DemoRoot) {
  const now = Date.now();
  const rows = call<{ programs: { startAt: string }[] }[]>(root, 'guide', {
    from: new Date(now).toISOString(),
    to: new Date(now + 6 * 3_600_000).toISOString(),
  });
  const recordings = call<{ edges: { node: { id: string } }[] }>(root, 'recordings', {
    first: 1,
    order: 'NEWEST_FIRST',
  });
  const watched = recordings.edges[0]?.node;

  return {
    nextEpisodes: () =>
      rows
        .flatMap((row) => row.programs)
        .filter((program) => Date.parse(program.startAt) > now)
        .slice(0, 3)
        .map((program) => ({ watched, program })),
  };
}

// 見たシリーズの続きの放送があるとき。
export const NextEpisodes: Story = { args: { overrides: withNextEpisodes } };

// 視聴者 (予約やルールを変えられない) で、続きの放送があるとき。予約のボタンを出さない。
export const NextEpisodesAsViewer: Story = {
  args: { overrides: merge(withNextEpisodes, asViewer('VIEWER')) },
};

// 録画も予約もなく、今放送している番組もないとき (入れたばかりのサーバー)。
export const Empty: Story = {
  args: {
    overrides: {
      recordings: emptyConnection,
      continueWatching: () => [],
      reservations: emptyConnection,
      guide: () => [],
    },
  },
};

// 続きから見る録画がないとき。
export const NoContinueWatching: Story = { args: { overrides: { continueWatching: () => [] } } };
