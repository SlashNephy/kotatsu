import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { holdAll, PageStory } from '../PageStory';

// ログイン中の端末と連携 (/settings/sessions) の画面 (#2056)。ほかの端末の有無、まとめて取り消す操作、連携中のアプリの有無を並べる。
const meta = {
  title: 'ページ/設定/セッション',
  component: PageStory,
  args: { url: '/settings/sessions' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const at = (day: number) => `2026-11-${String(day).padStart(2, '0')}T12:00:00+09:00`;

const current = {
  id: 'Session:1',
  current: true,
  createdAt: at(20),
  lastSeenAt: at(22),
  userAgent:
    'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/158.0 Safari/537.36',
  sourceIp: '192.0.2.10',
};

const others = [
  {
    id: 'Session:2',
    current: false,
    createdAt: at(10),
    lastSeenAt: at(21),
    userAgent:
      'Mozilla/5.0 (iPhone; CPU iPhone OS 19_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Mobile/15E148 Safari/604.1',
    sourceIp: '198.51.100.20',
  },
  // ユーザーエージェントを送らなかった端末。
  {
    id: 'Session:3',
    current: false,
    createdAt: at(1),
    lastSeenAt: at(2),
    userAgent: '',
    sourceIp: '203.0.113.30',
  },
];

const grants = [
  {
    id: 'OAuthGrant:1',
    clientId: 'claude',
    clientName: 'Claude',
    scopes: ['read', 'write'],
    createdAt: at(5),
    lastUsedAt: at(21),
  },
];

const base = {
  sessions: () => [current, ...others],
  oauthGrants: () => grants,
  revokeSession: ({ id }: { id: string }) => id,
  revokeOtherSessions: () => others.length,
};

export const Default: Story = { args: { overrides: base } };

export const Loading: Story = { args: { hold: holdAll } };

// この端末だけがログインしていて、連携中のアプリもないとき。
export const OnlyThisSession: Story = {
  args: { overrides: { ...base, sessions: () => [current], oauthGrants: () => [] } },
};

// ほかの端末をまとめて取り消す前の確認。
export const ConfirmRevokeOthers: Story = {
  args: { overrides: base },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: /ほかの端末/ }));
  },
};

// ほかの端末をまとめて取り消した後の通知。
export const RevokedOthers: Story = {
  args: { overrides: base },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: /ほかの端末/ }));
    const buttons = await canvas.findAllByRole('button', { name: /ほかの端末/ });
    const confirm = buttons.at(-1);
    if (confirm !== undefined) {
      await userEvent.click(confirm);
    }
  },
};
