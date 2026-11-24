import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { replaceProperty } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// 2 段階認証 (/login/second-factor) の画面 (#2056)。登録した第二要素 (パスキー、認証アプリ、リカバリーコード) ごとの表示、
// パスキーを使えない端末、コードが違うとき、待っているログインが切れたときを並べる。
const meta = {
  title: 'ページ/ログイン/2段階認証',
  component: PageStory,
  args: { url: '/login/second-factor' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// withMethods は、パスワードを検証した応答が、登録した第二要素として methods を返したときの URL。
const withMethods = (...methods: string[]) =>
  `/login/second-factor?methods=${encodeURIComponent(JSON.stringify(methods))}`;

// unwrap は、rejectAuth で包んだ fetch を戻す。
let unwrap = () => {};

// rejectAuth は、POST で path を呼んだら、エラー code を返すよう、fetch を包む。デモの応答 (interceptFetch) は成功の応答しか
// 返せないので、その外側を包む。描いた後 (play の中) に入れ、片付けは beforeEach の戻り値 (unwrapAuth) で行う。
function rejectAuth(path: string, code: string, status = 400) {
  const inner = window.fetch;
  const wrapped: typeof window.fetch = (input, init) => {
    const url = new URL(input instanceof Request ? input.url : input, window.location.href);
    if (url.pathname !== path || init?.method !== 'POST') {
      return inner(input, init);
    }

    return Promise.resolve(
      new Response(JSON.stringify({ error: { code } }), {
        status,
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

// enterCode は、認証アプリのコードを入れて、ログインを押す。コードの入力欄は 1 桁ずつの欄なので、最初の欄から続けて打つ。
const enterCode: Story['play'] = async ({ canvas, canvasElement }) => {
  await canvas.findByRole('button', { name: 'ログイン' });
  const [first] = canvasElement.querySelectorAll<HTMLInputElement>('input[inputmode="numeric"]');
  if (first !== undefined) {
    await userEvent.click(first);
    await userEvent.keyboard('123456');
  }
  await userEvent.click(await canvas.findByRole('button', { name: 'ログイン' }));
};

const noWebAuthn = () => replaceProperty(window, 'PublicKeyCredential', undefined);

// 既定。画面を直接開いたので、登録した第二要素が分からず、認証アプリのコードの入力を出す。
export const Default: Story = {};

// GraphQL を読まない画面なので、デモの応答を待たせても変わらない。
export const Loading: Story = { args: { hold: holdAll } };

// パスキーだけを登録しているとき (コードの入力を出さない)。
export const PasskeyOnly: Story = { args: { url: withMethods('passkey') } };

// パスキーと認証アプリを登録しているとき (どちらでも検証できる)。
export const PasskeyAndCode: Story = {
  args: { url: withMethods('passkey', 'totp', 'recovery_code') },
};

// パスキーを登録しているが、この端末で使えないとき。
export const PasskeyUnavailable: Story = {
  args: { url: withMethods('passkey', 'totp') },
  beforeEach: noWebAuthn,
};

// パスキーでの確認を始められなかったとき。
export const PasskeyFailed: Story = {
  args: {
    url: withMethods('passkey'),
    auth: { '/internal/auth/login/passkey': new Error('the demo server failed') },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'パスキーで確認' }));
  },
};

// リカバリーコードを使うとき。
export const RecoveryCode: Story = {
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'リカバリーコードを使う' }));
  },
};

// 入れたコードが違うとき。
export const InvalidCode: Story = {
  beforeEach: unwrapAuth,
  play: async (context) => {
    rejectAuth('/internal/auth/login/totp', 'INVALID_SECOND_FACTOR');
    await enterCode(context);
  },
};

// 待っているログインが切れていたとき (パスワードの入力に戻す)。
export const LoginExpired: Story = {
  beforeEach: unwrapAuth,
  play: async (context) => {
    rejectAuth('/internal/auth/login/totp', 'LOGIN_REQUIRED', 401);
    await enterCode(context);
  },
};
