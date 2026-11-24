import type { Meta, StoryObj } from '@storybook/react-vite';
import { expect, userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { paginate } from '../../demo/data';
import { asViewer, call, recordingsWith } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// 録画の一覧 (/recordings) の画面 (#2056)。録画のカードの状態のマーク、ルールで絞った一覧、まとめて消す操作などを並べる。
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

// カードのマークの出し分け。新しい順に、録画中、ドロップあり、一部のみ、失敗、スクランブル、CM の見分けの注記、サムネイルなし、
// 見かけ (続きから見る位置あり) の録画を並べる。
// card は、カードのマークを 1 つだけ見せるよう、デモの録画が持つマーク (ドロップ、保護、続きの位置) を外してから patch を当てる。
const card = (patch: Record<string, unknown>) => ({
  protected: false,
  drops: 0,
  errors: 0,
  watchPositionMs: null,
  ...patch,
});

// partial は、放送の途中の 15 分から、終わる 10 分前までだけ録れた録画にする。
const partial = (recording: Record<string, unknown>) => ({
  ...card({ state: 'PARTIAL' }),
  recordedFrom: new Date(Date.parse(String(recording['startAt'])) + 15 * 60_000).toISOString(),
  recordedUntil: new Date(Date.parse(String(recording['endAt'])) - 10 * 60_000).toISOString(),
});

export const CardStates: Story = {
  args: {
    overrides: recordingsWith(
      card({ state: 'RECORDING', recordedUntil: null }),
      card({ state: 'COMPLETED_WITH_ERRORS', drops: 128, errors: 12 }),
      partial,
      card({
        state: 'FAILED',
        recordedUntil: null,
        failureReason: 'TUNER_UNAVAILABLE',
        thumbnailUrl: null,
        previewUrl: null,
      }),
      card({
        state: 'COMPLETED_WITH_ERRORS',
        scrambled: 900_000,
        scrambleWarning: true,
        scrambledRatio: 0.9,
      }),
      card({ state: 'COMPLETED', cmDetectionNote: '映像のない区間がありました' }),
      card({ state: 'COMPLETED', thumbnailUrl: null, previewUrl: null, watchFrame: null }),
      card({ state: 'COMPLETED', watchPositionMs: 20 * 60_000 }),
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
    // 確認のダイアログは、Story の外 (ポータル) に出る。
    const dialog = await within(document.body).findByRole('dialog');
    await userEvent.click(await within(dialog).findByRole('button', { name: '選んだ1件を削除' }));
    await expect(await canvas.findByRole('status')).toBeVisible();
  },
};
