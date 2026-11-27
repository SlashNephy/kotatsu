import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { replaceProperty } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// ログイン (/login) の画面 (#2056)。初回セットアップの直後、OIDC で戻ってきたときの失敗、パスワードやパスキーや OIDC での
// ログインの失敗、パスキーを使えない端末、OIDC のボタンの有無、ログインの次の画面を並べる。
const meta = {
  title: 'ページ/ログイン',
  component: PageStory,
  args: { url: '/login' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// unwrap は、rejectAuth で包んだ fetch を戻す。
let unwrap = () => {};

// rejectAuth は、method で path を呼んだら、エラー code (と理由 reason) を返すよう、fetch を包む。デモの応答 (interceptFetch) は
// 成功の応答しか返せないので、その外側を包む。描いた後 (play の中) に入れ、片付けは beforeEach の戻り値 (unwrapAuth) で行う。
function rejectAuth(
  path: string,
  code: string,
  { status = 400, method = 'POST', reason, retryAfter }: RejectOptions = {},
) {
  const inner = window.fetch;
  const wrapped: typeof window.fetch = (input, init) => {
    const url = new URL(input instanceof Request ? input.url : input, window.location.href);
    if (url.pathname !== path || (init?.method ?? 'GET') !== method) {
      return inner(input, init);
    }
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (retryAfter !== undefined) {
      headers['Retry-After'] = String(retryAfter);
    }

    return Promise.resolve(
      new Response(JSON.stringify({ error: { code, reason } }), { status, headers }),
    );
  };
  window.fetch = wrapped;
  unwrap = () => {
    if (window.fetch === wrapped) {
      window.fetch = inner;
    }
  };
}

type RejectOptions = { status?: number; method?: string; reason?: string; retryAfter?: number };

const unwrapAuth = () => () => {
  unwrap();
  unwrap = () => {};
};

const oidc = { enabled: true, displayName: '社内のアカウント', linked: false };

// signIn は、ユーザー名とパスワードを入れて、ログインを押す。
const signIn: Story['play'] = async ({ canvas }) => {
  await userEvent.type(await canvas.findByLabelText(/ユーザー名/), 'demo');
  await userEvent.type(await canvas.findByLabelText(/パスワード/), 'correct horse battery');
  await userEvent.click(await canvas.findByRole('button', { name: 'ログイン' }));
};

export const Default: Story = {};

// GraphQL を読まない画面なので、デモの応答を待たせても変わらない。
export const Loading: Story = { args: { hold: holdAll } };

// 初回セットアップで管理者を作った直後。
export const AfterSetup: Story = { args: { url: '/login?setup=true' } };

// OIDC でのログインを、IdP で拒否して戻ってきたとき。
export const OIDCDenied: Story = { args: { url: '/login?error=oidc_denied' } };

// OIDC でのログインに、知らない理由で失敗して戻ってきたとき。
export const OIDCUnknownError: Story = { args: { url: '/login?error=unexpected' } };

// ユーザー名かパスワードが違うとき。
export const InvalidCredentials: Story = {
  beforeEach: unwrapAuth,
  play: async (context) => {
    rejectAuth('/internal/auth/login', 'INVALID_CREDENTIALS', { status: 401 });
    await signIn(context);
  },
};

// 試行が多すぎて、しばらく待たされるとき。
export const TooManyAttempts: Story = {
  beforeEach: unwrapAuth,
  play: async (context) => {
    rejectAuth('/internal/auth/login', 'TOO_MANY_ATTEMPTS', { status: 429, retryAfter: 30 });
    await signIn(context);
  },
};

// サーバーに届かなかったとき (理由の分からない失敗)。
export const LoginFailed: Story = {
  args: { auth: { '/internal/auth/login': new Error('the demo server failed') } },
  play: signIn,
};

// パスワードを確認し、第二要素の確認に進んだとき。
export const ToSecondFactor: Story = { play: signIn };

// パスワードを確認し、第二要素の登録に進んだとき (第二要素を 1 つも登録していない)。
export const ToEnroll: Story = {
  args: {
    auth: {
      '/internal/auth/login': {
        next: 'enroll_second_factor',
        methods: [],
        expiresAt: '2099-01-01T00:00:00Z',
      },
    },
  },
  play: signIn,
};

// この端末でパスキーを使えないとき (WebAuthn のないブラウザ)。
export const PasskeyUnavailable: Story = {
  beforeEach: () => replaceProperty(window, 'PublicKeyCredential', undefined),
};

// パスキーでのログインを始められなかったとき。
export const PasskeyFailed: Story = {
  args: { auth: { '/internal/auth/passkey/login': new Error('the demo server failed') } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'パスキーでログイン' }));
  },
};

// OIDC でのログインを受け付けているとき (IdP のボタン)。
export const OIDC: Story = { args: { auth: { '/internal/auth/oidc': oidc } } };

// IdP へ送り出せなかったとき。
export const OIDCBeginFailed: Story = {
  args: { auth: { '/internal/auth/oidc': oidc } },
  beforeEach: unwrapAuth,
  play: async ({ canvas }) => {
    rejectAuth('/internal/auth/oidc', 'OIDC_UNAVAILABLE', { status: 503 });
    await userEvent.click(
      await canvas.findByRole('button', { name: '社内のアカウントでログイン' }),
    );
  },
};
