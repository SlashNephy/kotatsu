import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { GraphQLRequestError } from '../../relay/environment';
import { call } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// 視聴ルールの編集 (/rules/watch/$watchRuleId) の画面 (#2056)。デモの最初の状態には視聴ルールがないので、
// Story を開く前に 1 つ作り (WatchRule:1)、それを開く。新しい視聴ルール、見つからないとき、保存と削除の結果も並べる。
const meta = {
  title: 'ページ/ルール/視聴ルール',
  component: PageStory,
  args: { url: '/rules/watch/WatchRule:1', overrides: withWatchRule },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// withWatchRule は、視聴ルールを 1 つ作る。デモの視聴ルールの ID は、作った順に WatchRule:1 から振る。
function withWatchRule(root: DemoRoot) {
  call(root, 'createWatchRule', { input: watchRuleInput('縁側の午後') });

  return {};
}

function watchRuleInput(name: string) {
  return {
    name,
    enabled: true,
    conditions: { keywords: [{ text: '縁側の午後', fields: ['NAME'] }] },
  };
}

// rejecting は、名前が WatchRuleIdQuery の読み込みを、error で失敗させる。hold の中で投げると、Relay に失敗として届く。
const rejecting = (error: () => unknown) => (op: { name: string }) => {
  if (op.name === 'WatchRuleIdQuery') {
    throw error();
  }

  return false;
};

const body = within(document.body);

// clickDelete は、視聴ルールを消すボタンを押す。キーワードの行を消すボタン (アイコンだけで、名前が同じ「削除」) は押さない。
async function clickDelete(canvas: { findAllByRole: typeof body.findAllByRole }) {
  const buttons = await canvas.findAllByRole('button', { name: '削除' });
  const button = buttons.find((candidate) => candidate.textContent?.trim() === '削除');
  if (button === undefined) {
    throw new Error('the story could not find the delete button');
  }
  await userEvent.click(button);
}

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 視聴ルールが見つからないとき (消した視聴ルールの URL を開いたなど)。
export const NotFound: Story = { args: { url: '/rules/watch/WatchRule:missing' } };

// サーバーが、視聴ルールが見つからない (NOT_FOUND) と応えたとき。
export const NotFoundByServer: Story = {
  args: { hold: rejecting(() => new GraphQLRequestError('not found', 404, 'NOT_FOUND')) },
};

// 視聴ルールを読み込めなかったとき。
export const LoadError: Story = {
  args: { hold: rejecting(() => new globalThis.Error('the demo server failed')) },
};

// ログインが切れていて、視聴ルールを読めなかったとき。
export const SignedOut: Story = {
  args: {
    hold: rejecting(() => new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED')),
  },
};

// 新しい視聴ルール。条件がないので保存できず、プレビューも取らない。削除のボタンは出さない。
export const New: Story = { args: { url: '/rules/watch/new' } };

// fillNew は、新しい視聴ルールに名前とキーワードを入れて、保存を押す。
async function fillNew(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await userEvent.type(await canvas.findByRole('textbox', { name: 'タイトル' }), '縁側の午後');
  // 新しいルールには、キーワードの行がない。含めるキーワードの欄 (最初の「キーワードを追加」) に行を足す。
  const [addKeyword] = canvas.getAllByRole('button', { name: 'キーワードを追加' });
  await userEvent.click(addKeyword!);
  const keyword = await canvas.findByRole('textbox', { name: '含めるキーワード' });
  await userEvent.type(keyword, '縁側の午後');
  await userEvent.click(canvas.getByRole('button', { name: '保存' }));
}

// 視聴ルールが上限まであって、作れなかったとき (CONFLICT)。
export const LimitReached: Story = {
  args: {
    url: '/rules/watch/new',
    overrides: () => ({ createWatchRule: failingWith({ code: 'CONFLICT' }) }),
  },
  play: async ({ canvasElement }) => fillNew(canvasElement),
};

// 新しい視聴ルールを作れなかったとき (上限でない失敗)。
export const CreateError: Story = {
  args: { url: '/rules/watch/new', overrides: () => ({ createWatchRule: failing() }) },
  play: async ({ canvasElement }) => fillNew(canvasElement),
};

// 保存したとき。
export const Saved: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '保存' }));
  },
};

// 保存できなかったとき。
export const SaveError: Story = {
  args: {
    overrides: (root: DemoRoot) => ({ ...withWatchRule(root), updateWatchRule: failing() }),
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '保存' }));
  },
};

// 視聴ルールを消す前の確かめ。
export const DeleteConfirm: Story = {
  play: async ({ canvas }) => {
    await clickDelete(canvas);
  },
};

// 視聴ルールを消せなかったとき。
export const DeleteError: Story = {
  args: {
    overrides: (root: DemoRoot) => ({ ...withWatchRule(root), deleteWatchRule: failing() }),
  },
  play: async ({ canvas }) => {
    await clickDelete(canvas);
    const dialog = within(await body.findByRole('dialog'));
    await userEvent.click(dialog.getByRole('button', { name: '削除' }));
  },
};

// captured は、Outdated の Story で作ったデモの既定の応答。play の中から、ほかの端末での変更を起こすのに使う。
// StrictMode で環境が 2 度作られても届くよう、作ったものをすべて覚える。
const captured: DemoRoot[] = [];

// 入力を触った後に、ほかの端末で視聴ルールが変わったとき。入力を残し、読み直すボタンを出す。
export const Outdated: Story = {
  args: {
    overrides: (root: DemoRoot) => {
      captured.push(root);

      return withWatchRule(root);
    },
  },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByRole('textbox', { name: 'タイトル' }), '（編集中）');
    for (const root of captured) {
      call(root, 'updateWatchRule', {
        id: 'WatchRule:1',
        input: watchRuleInput('縁側の午後 (ほかの端末で変更)'),
      });
    }
    // ほかの端末での変更の知らせを受けて、読み直すまで少しかかる。
    await canvas.findByRole('button', { name: '最新の内容を読み込む' }, { timeout: 10_000 });
  },
};
