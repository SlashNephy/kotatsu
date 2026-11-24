import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { holdAll, PageStory } from '../PageStory';

// 初期設定 (/setup) の画面 (#2056)。パスワードの強さ (弱い、強い)、セットアップトークンが違うとき、パスワードが規定に
// 当たったとき、管理者を作ってログインの画面へ進んだときを並べる。
const meta = {
  title: 'ページ/初期設定',
  component: PageStory,
  args: { url: '/setup' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// unwrap は、rejectAuth で包んだ fetch を戻す。
let unwrap = () => {};

// rejectAuth は、POST で path を呼んだら、エラー code (と理由 reason) を返すよう、fetch を包む。デモの応答 (interceptFetch) は
// 成功の応答しか返せないので、その外側を包む。描いた後 (play の中) に入れ、片付けは beforeEach の戻り値 (unwrapAuth) で行う。
function rejectAuth(path: string, code: string, reason?: string) {
  const inner = window.fetch;
  const wrapped: typeof window.fetch = (input, init) => {
    const url = new URL(input instanceof Request ? input.url : input, window.location.href);
    if (url.pathname !== path || init?.method !== 'POST') {
      return inner(input, init);
    }

    return Promise.resolve(
      new Response(JSON.stringify({ error: { code, reason } }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
  };
  window.fetch = wrapped;
  unwrap = () => {
    if (window.fetch === wrapped) {
      window.fetch = inner;
    }
  };
}

const unwrapAuth = () => () => {
  unwrap();
  unwrap = () => {};
};

// fill は、セットアップトークン、ユーザー名、表示名、パスワードを入れ、免責事項に同意する。submit なら、管理者を作るを押す。
const fill =
  (password: string, submit: boolean): Story['play'] =>
  async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText(/セットアップトークン/), 'demo-setup-token');
    await userEvent.type(await canvas.findByLabelText(/ユーザー名/), 'admin');
    await userEvent.type(await canvas.findByLabelText(/表示名/), '管理者');
    await userEvent.type(await canvas.findByLabelText(/パスワード/), password);
    if (submit) {
      await userEvent.click(await canvas.findByLabelText(/免責事項に同意します/));
      await userEvent.click(await canvas.findByRole('button', { name: '管理者を作る' }));
    }
  };

const strong = 'violet-harbor-lantern-1987';

export const Default: Story = {};

// GraphQL を読まない画面なので、デモの応答を待たせても変わらない。
export const Loading: Story = { args: { hold: holdAll } };

// 弱いパスワードを入れたとき (サーバーが断る強さで、修正方法の案内を出す)。
export const WeakPassword: Story = { play: fill('password', false) };

// 強いパスワードを入れたとき。
export const StrongPassword: Story = { play: fill(strong, false) };

// セットアップトークンが違うとき。
export const InvalidToken: Story = {
  beforeEach: unwrapAuth,
  play: async (context) => {
    rejectAuth('/internal/auth/setup', 'INVALID_SETUP_TOKEN');
    await fill(strong, true)?.(context);
  },
};

// パスワードが、漏れたパスワードの一覧に載っていたとき (サーバーが返した理由を出す)。
export const PasswordRejected: Story = {
  beforeEach: unwrapAuth,
  play: async (context) => {
    rejectAuth('/internal/auth/setup', 'WEAK_PASSWORD', 'breached');
    await fill(strong, true)?.(context);
  },
};

// 管理者を作り、ログインの画面へ進んだとき。
export const Done: Story = { play: fill(strong, true) };
