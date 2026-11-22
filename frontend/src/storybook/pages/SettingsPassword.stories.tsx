import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { failingWith, holdAll, PageStory } from '../PageStory';

// パスワードの変更 (/settings/password) の画面 (#2056)。強さの表示、変えた後、断られた理由ごとの表示、再認証を並べる。
const meta = {
  title: 'ページ/設定/パスワード',
  component: PageStory,
  args: { url: '/settings/password', overrides: { changePassword: () => true } },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// type は、新しいパスワードを打ち込む。submit なら、変えるボタンを押す。
const type =
  (password: string, submit: boolean): Story['play'] =>
  async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText(/新しいパスワード/), password);
    if (submit) {
      await userEvent.click(await canvas.findByRole('button', { name: /変更/ }));
    }
  };

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 打ち込んでいる途中 (強さの表示)。
export const WeakPassword: Story = { play: type('password', false) };

export const StrongPassword: Story = { play: type('kotatsu-mikan-neko-2026', false) };

// 変えたとき。
export const Changed: Story = { play: type('kotatsu-mikan-neko-2026', true) };

// 断られた理由ごとの表示。
const rejected = (reason: string): Story => ({
  args: { overrides: { changePassword: failingWith({ code: 'BAD_USER_INPUT', reason }) } },
  play: type('kotatsu-mikan-neko-2026', true),
});

export const RejectedTooShort = rejected('too_short');
export const RejectedTooLong = rejected('too_long');
export const RejectedBlocklisted = rejected('blocklisted');
export const RejectedBreached = rejected('breached');
export const RejectedTooWeak = rejected('too_weak');

// 画面を開いている間に、セッションが切れたとき。
export const Unauthenticated: Story = {
  args: { overrides: { changePassword: failingWith({ code: 'UNAUTHENTICATED' }) } },
  play: type('kotatsu-mikan-neko-2026', true),
};

// 分からない理由で失敗したとき。
export const UnknownError: Story = {
  args: { overrides: { changePassword: failingWith({ code: 'INTERNAL' }) } },
  play: type('kotatsu-mikan-neko-2026', true),
};

// 開いたときに、しばらく再認証していなかったとき。確かめる方法 (第二要素、パスキー、IdP) ごとに、再認証の画面が変わる。
const stale = { '/internal/auth/reauth': { recent: false } };

// 確かめる方法が 1 つも登録されていないとき。
export const ReauthNoMethod: Story = { args: { auth: stale } };

// 認証アプリとリカバリーコードで確かめるとき。
export const ReauthWithTotp: Story = {
  args: {
    auth: stale,
    overrides: { secondFactors: () => ({ totp: true, remainingRecoveryCodes: 8 }) },
  },
};

// パスキーで確かめるとき。
export const ReauthWithPasskey: Story = {
  args: { auth: stale, overrides: { passkeys: () => [{ id: 'Passkey:1' }] } },
};

// IdP で確かめるとき。
export const ReauthWithOIDC: Story = {
  args: {
    auth: {
      ...stale,
      '/internal/auth/oidc': { enabled: true, displayName: '社内のアカウント', linked: true },
    },
  },
};

// 変えようとしたときに、再認証を求められたとき。
export const ReauthOnSubmit: Story = {
  args: { overrides: { changePassword: failingWith({ code: 'REAUTHENTICATION_REQUIRED' }) } },
  play: type('kotatsu-mikan-neko-2026', true),
};

// IdP での再認証から、失敗して戻ってきたとき (理由ごと)。
const oidcError = (reason: string): Story => ({
  args: { url: `/settings/password?error=${reason}` },
});

export const OIDCExpired = oidcError('oidc_expired');
export const OIDCInvalid = oidcError('oidc_invalid');
export const OIDCDenied = oidcError('oidc_denied');
export const OIDCUsernameTaken = oidcError('oidc_username_taken');
export const OIDCUnavailable = oidcError('oidc_unavailable');
export const OIDCReauthStale = oidcError('oidc_reauth_stale');
export const OIDCReauthMismatch = oidcError('oidc_reauth_mismatch');
export const OIDCLinkTaken = oidcError('oidc_link_taken');
export const OIDCLinkAlready = oidcError('oidc_link_already');
export const OIDCLinkMismatch = oidcError('oidc_link_mismatch');
export const OIDCUnknownReason = oidcError('oidc_something_new');
