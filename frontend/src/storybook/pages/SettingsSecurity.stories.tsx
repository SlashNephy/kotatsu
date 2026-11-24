import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { call, replaceProperty } from '../conditions';
import { failing, holdAll, PageStory } from '../PageStory';

// セキュリティの設定 (/settings/security) の画面 (#2056)。認証アプリ、パスキー、API トークン、予約のカレンダー、IPTV の配信、
// OIDC の紐付けの、それぞれの状態と操作の途中を並べる。
const meta = {
  title: 'ページ/設定/セキュリティ',
  component: PageStory,
  args: { url: '/settings/security' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const at = (day: number) => `2026-11-${String(day).padStart(2, '0')}T12:00:00+09:00`;

// base は、どれも使っていない状態。管理者 (デモのユーザー) は、API トークンと IPTV の配信を有効にしている (デモの既定の設定)。
const base = {
  secondFactors: () => ({ totp: false, remainingRecoveryCodes: 0 }),
  passkeys: () => [],
  calendarFeed: () => null,
  iptvFeed: () => null,
  apiTokens: () => [],
  beginTotpEnrollment: () => ({
    secret: 'KOTATSUDEMOSECRET',
    uri: 'otpauth://totp/Kotatsu:demo?secret=KOTATSUDEMOSECRET&issuer=Kotatsu',
  }),
  confirmTotpEnrollment: () => ({
    codes: ['kotatsu-0001', 'kotatsu-0002', 'kotatsu-0003', 'kotatsu-0004'],
  }),
  createApiToken: ({ name }: { name: string }) => ({
    token: 'kotatsu_demo_0123456789abcdef',
    apiToken: { id: 'ApiToken:new', name, createdAt: at(22), lastUsedAt: null },
  }),
  createCalendarFeed: () => ({
    path: '/feeds/calendar/demo.ics',
    calendarFeed: { createdAt: at(22), lastUsedAt: null },
  }),
  createIptvFeed: () => ({
    playlistPath: '/feeds/iptv/demo.m3u',
    guidePath: '/feeds/iptv/demo.xml',
    iptvFeed: { createdAt: at(22), lastUsedAt: null },
  }),
};

const used = { createdAt: at(1), lastUsedAt: at(21) };

// features は、base に、管理者が API と IPTV の配信を有効にしているかを重ねる。そのほかの全体の設定は、デモの既定の値を使う。
const features =
  (enabled: { iptvFeedsEnabled: boolean; epgstationApiEnabled: boolean }, patch: object = {}) =>
  (root: DemoRoot) => ({
    ...base,
    ...patch,
    systemSettings: () => ({ ...call<object>(root, 'systemSettings'), ...enabled }),
  });

export const Default: Story = { args: { overrides: base } };

export const Loading: Story = { args: { hold: holdAll } };

// 認証アプリを登録し、リカバリーコードが残っているとき。
export const TotpEnabled: Story = {
  args: {
    overrides: { ...base, secondFactors: () => ({ totp: true, remainingRecoveryCodes: 8 }) },
  },
};

// しばらく再認証していないので、重要な操作の前に本人の確認を求めるとき。
export const ReauthRequired: Story = {
  args: {
    overrides: { ...base, secondFactors: () => ({ totp: true, remainingRecoveryCodes: 8 }) },
    auth: { '/internal/auth/reauth': { recent: false } },
  },
};

// 認証アプリを登録し始めたとき (QR コードとキー)。
export const TotpEnrolling: Story = {
  args: { overrides: base },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '認証アプリを登録する' }));
  },
};

// 認証アプリを登録し終え、リカバリーコードを出したとき。
export const RecoveryCodes: Story = {
  args: { overrides: base },
  play: async ({ canvas, canvasElement }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '認証アプリを登録する' }));
    await canvas.findByRole('button', { name: '登録する' });
    // コードの入力欄は 1 桁ずつの欄なので、最初の欄から続けて打つ。
    const [first] = canvasElement.querySelectorAll<HTMLInputElement>('input[inputmode="numeric"]');
    if (first !== undefined) {
      await userEvent.click(first);
      await userEvent.keyboard('123456');
    }
    await userEvent.click(await canvas.findByRole('button', { name: '登録する' }));
  },
};

// 操作に失敗したとき (認証アプリの登録を始められなかった)。
export const Failed: Story = {
  args: { overrides: { ...base, beginTotpEnrollment: failing() } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '認証アプリを登録する' }));
  },
};

const passkeys = [
  { id: 'Passkey:1', name: 'ノートパソコン', createdAt: at(1), lastUsedAt: at(21) },
  { id: 'Passkey:2', name: 'スマートフォン', createdAt: at(5), lastUsedAt: null },
];

// パスキーを登録しているとき。
export const Passkeys: Story = { args: { overrides: { ...base, passkeys: () => passkeys } } };

// パスキーの名前を変えているとき。
export const PasskeyRenaming: Story = {
  args: { overrides: { ...base, passkeys: () => passkeys } },
  play: async ({ canvas }) => {
    const [rename] = await canvas.findAllByRole('button', { name: '名前を変更' });
    if (rename !== undefined) {
      await userEvent.click(rename);
    }
  },
};

// この端末がパスキーを使えないとき (WebAuthn のないブラウザ)。
export const PasskeyUnavailable: Story = {
  args: { overrides: base },
  beforeEach: () => replaceProperty(window, 'PublicKeyCredential', undefined),
};

// API トークンを発行しているとき。
export const ApiTokens: Story = {
  args: {
    overrides: {
      ...base,
      apiTokens: () => [
        { id: 'ApiToken:1', name: 'スマートフォンのアプリ', ...used },
        { id: 'ApiToken:2', name: '使っていないトークン', createdAt: at(3), lastUsedAt: null },
      ],
    },
  },
};

// API トークンを発行した直後 (トークンを一度だけ出す)。
export const ApiTokenIssued: Story = {
  args: { overrides: base },
  play: async ({ canvas }) => {
    await userEvent.type(
      await canvas.findByRole('textbox', { name: '新しいAPIトークンの名前' }),
      'スマートフォン',
    );
    await userEvent.click(await canvas.findByRole('button', { name: 'APIトークンを発行する' }));
  },
};

// 管理者が EPGStation 互換の API を無効にしたが、発行済みのトークンがあるとき。
export const ApiTokensDisabled: Story = {
  args: {
    overrides: features(
      { iptvFeedsEnabled: true, epgstationApiEnabled: false },
      { apiTokens: () => [{ id: 'ApiToken:1', name: 'スマートフォンのアプリ', ...used }] },
    ),
  },
};

// 管理者が API と IPTV の配信を無効にし、発行済みのものもないとき (欄を出さない)。
export const FeaturesDisabled: Story = {
  args: {
    overrides: features({ iptvFeedsEnabled: false, epgstationApiEnabled: false }),
  },
};

// 予約のカレンダーと IPTV の配信の URL を発行済みのとき。
export const FeedsIssued: Story = {
  args: { overrides: { ...base, calendarFeed: () => used, iptvFeed: () => used } },
};

// 予約のカレンダーの URL を発行した直後 (URL を一度だけ出す)。
export const CalendarFeedCreated: Story = {
  args: { overrides: base },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '購読用のURLを発行' }));
  },
};

// IPTV の配信の URL を発行した直後。
export const IptvFeedCreated: Story = {
  args: { overrides: base },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'IPTVの配信のURLを発行' }));
  },
};

// 管理者が IPTV の配信を無効にしたが、発行済みの URL があるとき。
export const IptvFeedDisabled: Story = {
  args: {
    overrides: features(
      { iptvFeedsEnabled: false, epgstationApiEnabled: true },
      { iptvFeed: () => used },
    ),
  },
};

const oidc = { enabled: true, displayName: '社内のアカウント', linked: false };

// OIDC でのログインが有効で、まだ紐付けていないとき。
export const OIDCNotLinked: Story = {
  args: { overrides: base, auth: { '/internal/auth/oidc': oidc } },
};

// IdP で紐付けて戻ってきたとき。
export const OIDCLinked: Story = {
  args: {
    url: '/settings/security?linked=oidc',
    overrides: base,
    auth: { '/internal/auth/oidc': { ...oidc, linked: true } },
  },
};
