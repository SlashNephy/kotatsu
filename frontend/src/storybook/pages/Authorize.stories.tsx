import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// アプリとの連携の許可 (/authorize) の画面 (#2056)。許可を求めるアプリとスコープ (ロールで許さないスコープ、選べるスコープがない)、
// 本人の確認を先に求めるとき、正しくないリクエスト (戻す先の有無)、連携を無効にしているとき、許可に失敗したときを並べる。
const meta = {
  title: 'ページ/連携の許可',
  component: PageStory,
  args: { url: '/authorize' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const authorization = (allowedScopes: string[]) => ({
  authorization: {
    clientId: 'https://assistant.example.com/oauth/client.json',
    clientName: 'デモのアシスタント',
    clientUri: 'https://assistant.example.com/',
    redirectUri: 'https://assistant.example.com/oauth/callback',
    scopes: ['mcp:read', 'mcp:write'],
    allowedScopes,
  },
  errorRedirectUrl: null,
});

const consent = (allowedScopes: string[]) => ({
  oauthAuthorization: () => authorization(allowedScopes),
  secondFactors: () => ({ totp: true, remainingRecoveryCodes: 8 }),
  passkeys: () => [],
});

const approve: Story['play'] = async ({ canvas }) => {
  await userEvent.click(await canvas.findByRole('button', { name: '許可する' }));
};

// 既定。デモは認可のリクエストを確認しないので、正しくないリクエストとして出す (クライアントへ戻す先も分からない)。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 読み書きのスコープを求められ、どちらも許せるとき。
export const Consent: Story = { args: { overrides: consent(['mcp:read', 'mcp:write']) } };

// ロールで書き込みを許さないとき (書き込みのスコープを選べない)。
export const ScopeNotAllowed: Story = { args: { overrides: consent(['mcp:read']) } };

// 求められたスコープを 1 つも許せないとき (許可するを押せない)。
export const NoScopeAllowed: Story = { args: { overrides: consent([]) } };

// しばらく本人の確認をしていないので、スコープを選ぶ前に確認するとき。
export const SudoRequired: Story = {
  args: {
    overrides: consent(['mcp:read', 'mcp:write']),
    auth: { '/internal/auth/reauth': { recent: false, authenticatedAt: '2026-11-21T00:00:00Z' } },
  },
};

// 本人の確認で、パスキーと IdP (紐付け済み) も使えるとき。
export const SudoRequiredWithPasskeyAndOIDC: Story = {
  args: {
    overrides: {
      ...consent(['mcp:read', 'mcp:write']),
      passkeys: () => [{ id: 'Passkey:1' }],
    },
    auth: {
      '/internal/auth/reauth': { recent: false, authenticatedAt: '2026-11-21T00:00:00Z' },
      '/internal/auth/oidc': { enabled: true, displayName: '社内のアカウント', linked: true },
    },
  },
};

// 本人の確認に使える手段が 1 つもないとき (第二要素もパスキーも IdP もない)。
export const SudoUnavailable: Story = {
  args: {
    overrides: {
      ...consent(['mcp:read', 'mcp:write']),
      secondFactors: () => ({ totp: false, remainingRecoveryCodes: 0 }),
    },
    auth: { '/internal/auth/reauth': { recent: false, authenticatedAt: '2026-11-21T00:00:00Z' } },
  },
};

// 許可しようとしたら、本人の確認の期限が切れていたとき (確認する画面を出す)。
export const ReauthOnApprove: Story = {
  args: {
    overrides: {
      ...consent(['mcp:read', 'mcp:write']),
      approveOAuthAuthorization: failingWith({ code: 'REAUTHENTICATION_REQUIRED' }),
    },
  },
  play: approve,
};

// 許可できなかったとき。
export const ApproveFailed: Story = {
  args: {
    overrides: { ...consent(['mcp:read', 'mcp:write']), approveOAuthAuthorization: failing() },
  },
  play: approve,
};

// 正しくないリクエストで、エラーを付けてクライアントへ戻せるとき (戻す先のホストを見せ、押してから戻す)。
export const InvalidRequestWithRedirect: Story = {
  args: {
    overrides: {
      oauthAuthorization: () => ({
        authorization: null,
        errorRedirectUrl: 'https://assistant.example.com/oauth/callback?error=invalid_request',
      }),
    },
  },
};

// 管理者が連携を無効にしているとき。
export const Disabled: Story = {
  args: { overrides: { oauthAuthorization: failingWith({ code: 'NOT_FOUND' }) } },
};

// リクエストを確認できなかったとき (クライアントの文書を読めないなど)。
export const BadRequest: Story = {
  args: { overrides: { oauthAuthorization: failingWith({ code: 'BAD_USER_INPUT' }) } },
};

// そのほかの理由で読み込めなかったとき。
export const Failed: Story = { args: { overrides: { oauthAuthorization: failing() } } };
