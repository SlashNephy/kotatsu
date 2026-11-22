import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { asViewer, merge } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// 通信の記録 (/network-activity) の画面 (#2056)。記録していないとき、記録していて通信がないとき、日ごとの通信の一覧
// (失敗した通信、宛先を伏せた通信)、管理者でないとき、切り替えに失敗したときの表示を並べる。
const meta = {
  title: 'ページ/通信の記録',
  component: PageStory,
  args: { url: '/network-activity' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const hoursAgo = (hours: number) =>
  new Date(Math.floor(Date.now() / 3_600_000 - hours) * 3_600_000).toISOString();

const entry = (
  hours: number,
  purpose: string,
  host: string | null,
  fields: Partial<{
    initiator: string;
    success: boolean;
    count: number;
    receivedBytes: number;
  }> = {},
) => ({
  hour: hoursAgo(hours),
  purpose,
  host,
  initiator: 'SCHEDULED',
  success: true,
  count: 1,
  sentBytes: 2_048,
  receivedBytes: 48_000,
  ...fields,
});

const recorded = {
  enabled: true,
  enabledAt: '2026-11-01T09:00:00+09:00',
  retentionDays: 30,
  totalCount: 42,
  totalSentBytes: 120_000,
  totalReceivedBytes: 35_000_000,
};

const activity = () => ({
  ...recorded,
  entries: [
    entry(1, 'JIKKYO_COMMENTS', 'jikkyo.example.com', {
      initiator: 'USER',
      count: 12,
      receivedBytes: 3_500_000,
    }),
    entry(2, 'UPDATE_CHECK', 'api.example.com', { success: false }),
    // Webhook の宛先は、利用者の設定なので伏せる。
    entry(3, 'WEBHOOK', null, { initiator: 'USER' }),
    // 宛先を伏せる通信のうち、伏せた理由の文を持たないもの。
    entry(4, 'WEB_PUSH', null),
    entry(30, 'ANIME_PROGRAM_SYNC', 'calendar.example.com', { count: 24 }),
  ],
});

// 既定。管理者が記録を有効にしていないとき。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 読み込めなかったとき。
export const Error: Story = { args: { overrides: { networkActivity: failing() } } };

// 記録していて、日ごとの通信があるとき。失敗した通信と、宛先を伏せた通信を含む。
export const Recorded: Story = { args: { overrides: { networkActivity: activity } } };

// 記録を有効にしたが、まだ通信がないとき。
export const NoEntries: Story = {
  args: { overrides: { networkActivity: () => ({ ...recorded, totalCount: 0, entries: [] }) } },
};

// 管理者でないとき (記録を切り替えるスイッチを出さない)。
export const AsMember: Story = {
  args: { overrides: merge(() => ({ networkActivity: activity }), asViewer('MEMBER')) },
};

// 記録の切り替えに失敗したとき。
export const ToggleFailed: Story = {
  args: { overrides: { updateSystemSettings: failing() } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('switch', { name: /外部との通信を記録する/ }));
  },
};
