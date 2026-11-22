import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { GraphQLRequestError } from '../../relay/environment';
import { asViewer, call } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// 番組検索 (/search) の画面 (#2056)。検索のキーワードは URL に載らないので、Story を開いた後に打ち込む。
const meta = {
  title: 'ページ/番組検索',
  component: PageStory,
  args: { url: '/search' },
  parameters: { layout: 'fullscreen' },
  play: async ({ canvasElement }) => search(canvasElement, 'こたつ'),
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// search は、含めるキーワードの欄に keyword を打ち込む。
async function search(canvasElement: HTMLElement, keyword: string) {
  await userEvent.type(
    await within(canvasElement).findByRole('textbox', { name: '含めるキーワード' }),
    keyword,
  );
}

// searchAndThen は、キーワードを打ち込み、結果が出たら then を行う。
function searchAndThen(keyword: string, then: (canvasElement: HTMLElement) => Promise<void>) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await search(canvasElement, keyword);
    await then(canvasElement);
  };
}

// firstResult は、検索の結果の最初の番組を返す。
async function firstResult(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const [result] = await canvas.findAllByRole('button', { name: /こたつ/ });

  return result!;
}

export const Default: Story = {};

// 読み込み中は、キーワードの入力欄が出ないので、打ち込まない。
export const Loading: Story = { args: { hold: holdAll }, play: async () => {} };

// 検索に失敗したとき。
export const Error: Story = { args: { overrides: { searchPrograms: failing() } } };

// ログインが切れていて、画面を読めなかったとき。hold の中で投げると、Relay に失敗として届く。
export const SignedOut: Story = {
  args: {
    hold: (operation) => {
      if (operation.name === 'searchViewerQuery') {
        throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
      }

      return false;
    },
  },
  play: async () => {},
};

// 条件をまだ入れていないとき。探さず、ルールを作るボタンも押せない。
export const NoConditions: Story = { play: async () => {} };

// 検索している最中。結果の見出しの横に、読み込み中を出す。
export const Searching: Story = {
  args: { hold: (operation) => operation.name === 'searchProgramsQuery' },
};

// 当たる番組がないとき。
export const NoResults: Story = {
  play: async ({ canvasElement }) => search(canvasElement, '見つからない語'),
};

// 結果が 1 回に取る件数より多いとき。続きを読み込むボタンを出す。
export const HasMore: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      searchPrograms: (args: { first: number } & Record<string, unknown>) =>
        call(root, 'searchPrograms', { ...args, first: Math.min(args.first, 3) }),
    }),
  },
};

// 続きを読み込めなかったとき。
export const MoreError: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      searchPrograms: (
        args: { first: number; after?: string | null } & Record<string, unknown>,
      ) => {
        if (args.after != null) {
          throw new globalThis.Error('the demo server failed');
        }

        return call(root, 'searchPrograms', { ...args, first: Math.min(args.first, 3) });
      },
    }),
  },
  play: searchAndThen('こたつ', async (canvasElement) => {
    await userEvent.click(
      await within(canvasElement).findByRole('button', { name: 'さらに読み込む' }),
    );
  }),
};

// 過去の放送も含めて探すとき (#1121)。探す日数の欄を出し、放送の終わった番組に印を付ける。
export const IncludeArchive: Story = {
  play: searchAndThen('こたつ', async (canvasElement) => {
    await userEvent.click(
      within(canvasElement).getByRole('switch', { name: '過去の放送を含める' }),
    );
  }),
};

// 視聴者から見たとき。ルールを作るボタンを出さない。
export const AsViewer: Story = { args: { overrides: asViewer('VIEWER') } };

// 結果の番組を押して、番組の詳細を開いたとき。
export const ProgramDetail: Story = {
  play: searchAndThen('こたつ', async (canvasElement) => {
    await userEvent.click(await firstResult(canvasElement));
  }),
};

// 検索の条件からルールを作ったとき。条件を引き継ぎ、最初のキーワードを名前にして、ルールの作成画面を開く。
export const CreateRule: Story = {
  play: searchAndThen('こたつ', async (canvasElement) => {
    await userEvent.click(
      within(canvasElement).getByRole('button', { name: 'この条件でルールを作成' }),
    );
  }),
};
