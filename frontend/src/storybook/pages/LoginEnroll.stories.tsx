import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { holdAll, PageStory } from '../PageStory';

// 認証アプリの登録 (/login/enroll) の画面 (#2056)。QR コードとキー、登録を始められなかったとき、コードが違うとき、
// 登録し終えてリカバリーコードを出したときを並べる。
const meta = {
  title: 'ページ/ログイン/認証アプリの登録',
  component: PageStory,
  args: { url: '/login/enroll' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// unwrap は、rejectAuth で包んだ fetch を戻す。
let unwrap = () => {};

// rejectAuth は、POST で path を呼んだら、エラー code を返すよう、fetch を包む。デモの応答 (interceptFetch) は成功の応答しか
// 返せないので、その外側を包む。描いた後 (play の中) に入れ、片付けは beforeEach の戻り値 (unwrapAuth) で行う。
function rejectAuth(path: string, code: string) {
  const inner = window.fetch;
  const wrapped: typeof window.fetch = (input, init) => {
    const url = new URL(input instanceof Request ? input.url : input, window.location.href);
    if (url.pathname !== path || init?.method !== 'POST') {
      return inner(input, init);
    }

    return Promise.resolve(
      new Response(JSON.stringify({ error: { code } }), {
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

// enterCode は、認証アプリのコードを入れて、登録するを押す。コードの入力欄は 1 桁ずつの欄なので、最初の欄から続けて打つ。
const enterCode: Story['play'] = async ({ canvas, canvasElement }) => {
  await canvas.findByRole('button', { name: '登録する' });
  const [first] = canvasElement.querySelectorAll<HTMLInputElement>('input[inputmode="numeric"]');
  if (first !== undefined) {
    await userEvent.click(first);
    await userEvent.keyboard('123456');
  }
  await userEvent.click(await canvas.findByRole('button', { name: '登録する' }));
};

// 既定。QR コードとキーを出し、コードの入力を待つ。
export const Default: Story = {};

// GraphQL を読まない画面なので、デモの応答を待たせても変わらない。
export const Loading: Story = { args: { hold: holdAll } };

// 登録を始められなかったとき。
export const BeginFailed: Story = {
  args: {
    auth: { '/internal/auth/login/totp/enrollment': new Error('the demo server failed') },
  },
};

// 入れたコードが違うとき。
export const InvalidCode: Story = {
  beforeEach: unwrapAuth,
  play: async (context) => {
    rejectAuth('/internal/auth/login/totp/enrollment/confirm', 'INVALID_SECOND_FACTOR');
    await enterCode(context);
  },
};

// 登録し終え、リカバリーコードを 1 回だけ出したとき。
export const RecoveryCodes: Story = {
  args: {
    auth: {
      '/internal/auth/login/totp/enrollment/confirm': {
        recoveryCodes: ['kotatsu-0001', 'kotatsu-0002', 'kotatsu-0003', 'kotatsu-0004'],
      },
    },
  },
  play: enterCode,
};
