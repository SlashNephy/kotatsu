import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { paginate } from '../../demo/data';
import { asViewer, call, minutesAgo, recordingsWith } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// 録画の一覧 (/recordings) の画面 (#2056)。録画のカードの状態の印、ルールで絞った一覧、まとめて消す操作などを並べる。
const meta = {
  title: 'ページ/録画',
  component: PageStory,
  args: { url: '/recordings' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const emptyRecordings = (args: { first: number; after?: string | null }) =>
  paginate([], args.first, args.after);

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 録画がまだ 1 つもないとき。
export const Empty: Story = { args: { overrides: { recordings: emptyRecordings } } };

export const Error: Story = { args: { overrides: { recordings: failing() } } };

// カードの印の出し分け。新しい順に、録画中、ドロップあり、一部のみ、失敗、スクランブル、CM の見分けの注記、サムネイルなし、
// 見かけ (続きから見る位置あり) の録画を並べる。
export const CardStates: Story = {
  args: {
    overrides: recordingsWith(
      { state: 'RECORDING', recordedUntil: null },
      { state: 'COMPLETED_WITH_ERRORS', drops: 128, errors: 12 },
      { state: 'PARTIAL', recordedFrom: minutesAgo(70), recordedUntil: minutesAgo(45) },
      { state: 'FAILED', failureReason: 'TUNER_UNAVAILABLE', thumbnailUrl: null, previewUrl: null },
      {
        state: 'COMPLETED_WITH_ERRORS',
        scrambled: 900_000,
        scrambleWarning: true,
        scrambledRatio: 0.9,
      },
      { state: 'COMPLETED', cmDetectionNote: '映像のない区間がありました' },
      { state: 'COMPLETED', thumbnailUrl: null, previewUrl: null, watchFrame: null },
      { state: 'COMPLETED', watchPositionMs: 20 * 60_000 },
    ),
  },
};

// ルールで絞った一覧 (ルールの画面から開いたとき)。
export const ByRule: Story = { args: { url: '/recordings?rule=Rule:1' } };

// ルールで絞って、一致する録画がないとき。
export const ByRuleNoMatch: Story = {
  args: { url: '/recordings?rule=Rule:1', overrides: { recordings: emptyRecordings } },
};

// 続きがあり、「さらに読み込む」を出すとき。
export const HasMore: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      recordings: (args: { first: number }) => ({
        ...call<object>(root, 'recordings', { ...args, first: 3 }),
        pageInfo: { hasNextPage: true, endCursor: '2', hasPreviousPage: false, startCursor: '0' },
      }),
    }),
  },
};

// 視聴者 (録画を消せない) から見たとき。まとめて消すボタンを出さない。
export const AsViewer: Story = { args: { overrides: asViewer('VIEWER') } };

// まとめて消す録画を選んでいるとき。
export const Selecting: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '選ぶ' }));
    // 録画中の録画は消せないので、選べる録画のうち最初のものを選ぶ。
    const selectable = (await canvas.findAllByRole('checkbox')).find(
      (checkbox) => checkbox.getAttribute('aria-disabled') !== 'true',
    );
    if (selectable !== undefined) {
      await userEvent.click(selectable);
    }
  },
};

// まとめて消した後の知らせ。
export const Deleted: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '選ぶ' }));
    // 録画中の録画は消せないので、選べる録画のうち最初のものを選ぶ。
    const selectable = (await canvas.findAllByRole('checkbox')).find(
      (checkbox) => checkbox.getAttribute('aria-disabled') !== 'true',
    );
    if (selectable !== undefined) {
      await userEvent.click(selectable);
    }
    await userEvent.click(await canvas.findByRole('button', { name: '選んだ1件を削除' }));
    // 確かめのダイアログは、Story の外 (ポータル) に出る。
    const dialog = await within(document.body).findByRole('dialog');
    await userEvent.click(await within(dialog).findByRole('button', { name: '選んだ1件を削除' }));
    await expect(await canvas.findByRole('status')).toBeVisible();
  },
};
