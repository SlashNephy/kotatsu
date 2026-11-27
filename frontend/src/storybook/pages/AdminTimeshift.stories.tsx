import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { call, minutesAgo } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// タイムシフトの設定 (/admin/timeshift) の画面 (#2056)。録るサービスの状態と、視聴中だけ録る設定を並べる。
const meta = {
  title: 'ページ/管理/タイムシフト',
  component: PageStory,
  args: { url: '/admin/timeshift' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// available は、タイムシフトのディレクトリがあり、録るサービスをまだ選んでいない状態。
const available = { timeshiftAvailable: () => true, timeshiftChannels: () => [] };

// channels は、録るサービスを 2 つ選んだ状態。1 つ目は録っていてバッファがあり、2 つ目はチューナーを譲って止まり、バッファがない。
// 2 つ目の保持時間は、時間単位でない値 (90 分) で、欄では近い時間に丸める。
const channels = (root: DemoRoot) => {
  const [first, second] = call<object[]>(root, 'services');

  return {
    timeshiftAvailable: () => true,
    timeshiftChannels: () => [
      {
        service: first,
        retentionSeconds: 7200,
        bufferedFrom: minutesAgo(120),
        bufferedUntil: minutesAgo(0),
        recording: true,
      },
      {
        service: second,
        retentionSeconds: 5400,
        bufferedFrom: null,
        bufferedUntil: null,
        recording: false,
      },
    ],
  };
};

// タイムシフトのディレクトリがなく、使えないとき (デモの既定)。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 管理者でないとき (サーバーが FORBIDDEN で拒んだ)。
export const Forbidden: Story = {
  args: { overrides: { timeshiftAvailable: failingWith({ code: 'FORBIDDEN' }) } },
};

// 設定を読めなかったとき。
export const Failed: Story = { args: { overrides: { timeshiftAvailable: failing() } } };

// 使えるが、録るサービスを選んでいないとき。
export const Empty: Story = { args: { overrides: available } };

// 録るサービスがあるとき (録画中でバッファのあるものと、停止中でバッファのないもの)。
export const Channels: Story = { args: { overrides: channels } };

// ライブ視聴している間だけ録るのを有効にしたとき。
export const WhileWatching: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      ...channels(root),
      systemSettings: () => ({
        ...call<object>(root, 'systemSettings'),
        timeshiftWhileWatching: true,
        timeshiftWhileWatchingRetentionSeconds: 3 * 3600,
      }),
    }),
  },
};

// サービスを外す前に確認するとき。
export const ConfirmRemove: Story = {
  args: { overrides: channels },
  play: async ({ canvas }) => {
    const [remove] = await canvas.findAllByRole('button', { name: '外す' });
    if (remove === undefined) {
      throw new Error('the story could not find remove');
    }
    await userEvent.click(remove);
  },
};

// 足すサービスを選び、追加のボタンを押せるようになったとき。
export const ServiceSelected: Story = {
  args: { overrides: available },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByLabelText('サービス'));
    const [option] = await within(document.body).findAllByRole('option');
    if (option === undefined) {
      throw new Error('the story could not find option');
    }
    await userEvent.click(option);
  },
};

// サービスを足せなかったとき (失敗の通知を出し、選んだサービスを残す)。
export const AddFailed: Story = {
  args: { overrides: { ...available, setTimeshiftChannel: failing() } },
  play: async (context) => {
    await ServiceSelected.play?.(context);
    await userEvent.click(await context.canvas.findByRole('button', { name: '追加' }));
  },
};

// 足すサービスを選んだが、保持時間の欄を空にして、追加のボタンを押せないとき。
export const RetentionCleared: Story = {
  args: { overrides: available },
  play: async (context) => {
    await ServiceSelected.play?.(context);
    const fields = await context.canvas.findAllByRole('textbox', { name: '保持時間（時間）' });
    // 足す行の欄は、視聴中のサービスの欄より前にある。
    const field = fields.at(-2);
    if (field === undefined) {
      throw new Error('the story could not find the retention field');
    }
    await userEvent.clear(field);
  },
};
