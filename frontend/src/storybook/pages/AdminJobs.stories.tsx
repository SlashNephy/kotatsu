import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { asViewer, merge, minutesAgo } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// ジョブ (/admin/jobs) の画面 (#2056)。状態のタブと数、ジョブの一覧 (録画、進み具合、エラーの有無)、ジョブを開いたときの
// エラーと操作 (試し直す、取り消す、取り消せない)、操作の失敗を並べる。
const meta = {
  title: 'ページ/管理/ジョブ',
  component: PageStory,
  args: { url: '/admin/jobs' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const job = (patch: object) => ({
  queue: 'default',
  attempt: 1,
  maxAttempts: 25,
  createdAt: minutesAgo(30),
  scheduledAt: minutesAgo(30),
  attemptedAt: minutesAgo(10),
  finalizedAt: null,
  errors: [],
  recording: null,
  retryable: false,
  cancellable: true,
  progress: null,
  ...patch,
});

const failure = (attempt: number, minutes: number) => ({
  at: minutesAgo(minutes),
  attempt,
  message: 'ffmpeg: exit status 1\nInvalid data found when processing input',
});

// running は、動いているジョブ。録画のあるもの、進み具合と残り時間 (計算中、1 分未満、分、時間) のあるもの、名前のない種類、
// 前の試行で失敗したもの、取り消せないものを含む。
const running = [
  job({
    id: '101',
    kind: 'recording_transcode',
    state: 'RUNNING',
    recording: { id: 'Recording:demo-1', name: '架空のドラマ 第3話' },
    progress: { ratio: 0.427, remainingSeconds: 1500, updatedAt: minutesAgo(0) },
  }),
  job({
    id: '102',
    kind: 'library_cm_detect',
    state: 'RUNNING',
    progress: { ratio: 0.05, remainingSeconds: null, updatedAt: minutesAgo(0) },
  }),
  job({
    id: '105',
    kind: 'thumbnail_live',
    state: 'RUNNING',
    progress: { ratio: 0.98, remainingSeconds: 40, updatedAt: minutesAgo(0) },
  }),
  job({
    id: '106',
    kind: 'recording_join',
    state: 'RUNNING',
    progress: { ratio: 0.2, remainingSeconds: 5400, updatedAt: minutesAgo(0) },
  }),
  job({
    id: '103',
    kind: 'recording_analyze',
    state: 'RUNNING',
    attempt: 3,
    errors: [failure(1, 25), failure(2, 15)],
    recording: { id: 'Recording:demo-2', name: '架空のアニメ 第12話' },
  }),
  job({ id: '104', kind: 'custom_future_kind', state: 'RUNNING', cancellable: false }),
];

// retryable は、再試行を待つジョブ。すぐに試し直せる。
const retryable = [
  job({
    id: '201',
    kind: 'webhook_deliver',
    state: 'RETRYABLE',
    attempt: 2,
    scheduledAt: minutesAgo(-10),
    errors: [failure(1, 20), failure(2, 5)],
    retryable: true,
  }),
];

// completed は、終えたジョブ。取り消せず、試し直せる。
const completed = [
  job({
    id: '301',
    kind: 'thumbnail_generate',
    state: 'COMPLETED',
    finalizedAt: minutesAgo(3),
    retryable: true,
    cancellable: false,
  }),
];

const byState: Record<string, unknown[]> = {
  RUNNING: running,
  RETRYABLE: retryable,
  COMPLETED: completed,
};

const counts = () => [
  { state: 'RUNNING', count: running.length },
  { state: 'AVAILABLE', count: 12 },
  { state: 'SCHEDULED', count: 3 },
  { state: 'RETRYABLE', count: retryable.length },
  { state: 'COMPLETED', count: 1520 },
  { state: 'DISCARDED', count: 2 },
];

const jobs = {
  jobCounts: counts,
  jobs: ({ state }: { state: string }) => ({ nodes: byState[state] ?? [] }),
};

// ジョブがないとき (デモにはジョブがない)。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 動いているジョブの一覧と、状態ごとの数。
export const Running: Story = { args: { overrides: jobs } };

// エラーのあるジョブを開いたとき (試行ごとのエラー)。
export const OpenedWithErrors: Story = {
  args: { overrides: jobs },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByText('103'));
  },
};

// 取り消せない、終えていないジョブを開いたとき (エラーはなく、取り消せないことの案内を出す)。
export const OpenedUncancellable: Story = {
  args: { overrides: jobs },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByText('104'));
  },
};

// 再試行を待つジョブのタブ。開いて、すぐに試し直せる。
export const Retryable: Story = {
  args: { overrides: jobs },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('tab', { name: /再試行を待つ/ }));
    await userEvent.click(await canvas.findByText('201'));
  },
};

// 終えたジョブのタブ。終えた時刻を出し、取り消せない。
export const Completed: Story = {
  args: { overrides: jobs },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('tab', { name: /完了/ }));
    await userEvent.click(await canvas.findByText('301'));
  },
};

// 試し直せなかったとき。
export const RetryFailed: Story = {
  args: { overrides: { ...jobs, retryJob: failing() } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('tab', { name: /再試行を待つ/ }));
    await userEvent.click(await canvas.findByText('201'));
    await userEvent.click(await canvas.findByRole('button', { name: 'すぐに試し直す' }));
  },
};

// 管理者でないユーザーが開いたとき (サーバーが FORBIDDEN で断る)。
export const Forbidden: Story = {
  args: {
    overrides: merge(asViewer('MEMBER'), () => ({ jobCounts: failingWith({ code: 'FORBIDDEN' }) })),
  },
};
