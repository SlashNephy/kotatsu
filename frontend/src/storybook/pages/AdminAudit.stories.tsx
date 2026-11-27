import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { GraphQLRequestError } from '../../relay/requestError';
import { asViewer, merge, minutesAgo } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// 監査ログ (/admin/audit) の画面 (#2056)。記録の一覧、詳細を開いたとき、続きの読み込み、管理の画面の枠の出し分け
// (ログインしていない、管理者でない、読めない) を並べる。
const meta = {
  title: 'ページ/管理/監査ログ',
  component: PageStory,
  args: { url: '/admin/audit' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const users = () => [
  { id: 'User:1', username: 'admin', displayName: '管理者' },
  { id: 'User:2', username: 'family', displayName: '家族' },
];

type AuditEvent = {
  id: string;
  occurredAt: string;
  actor: {
    type: string;
    user: { id: string; username: string; displayName: string } | null;
    role: string;
  };
  action: string;
  targetType: string;
  targetId: string | null;
  summary: string;
  sourceIp: string | null;
  userAgent: string | null;
};

const admin = {
  type: 'user',
  user: { id: 'User:1', username: 'admin', displayName: '管理者' },
  role: 'ADMIN',
};

// events は、操作者と送信元の違う記録。ユーザーの操作、システムの操作、消したユーザーの操作を含む。
const events: AuditEvent[] = [
  {
    id: 'AuditEvent:1',
    occurredAt: minutesAgo(5),
    actor: admin,
    action: 'user.create',
    targetType: 'user',
    targetId: '2',
    summary: '{"username":"family","role":"MEMBER"}',
    sourceIp: '192.0.2.10',
    userAgent: 'Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0',
  },
  {
    id: 'AuditEvent:2',
    occurredAt: minutesAgo(30),
    actor: { type: 'system', user: null, role: 'nekonote' },
    action: 'storage.prune',
    targetType: 'storage',
    targetId: null,
    summary: 'removed 3 files',
    sourceIp: null,
    userAgent: null,
  },
  {
    id: 'AuditEvent:3',
    occurredAt: minutesAgo(120),
    actor: { type: 'user', user: null, role: 'MEMBER' },
    action: 'rule.delete',
    targetType: 'rule',
    targetId: '42',
    summary: '{}',
    sourceIp: null,
    userAgent: null,
  },
];

const older: AuditEvent[] = [
  {
    id: 'AuditEvent:4',
    occurredAt: minutesAgo(600),
    actor: admin,
    action: 'settings.update',
    targetType: 'system_settings',
    targetId: null,
    summary: '{"mcpEnabled":true}',
    sourceIp: '192.0.2.10',
    userAgent: null,
  },
];

const page = (nodes: AuditEvent[], next: string | null) => ({
  edges: nodes.map((node) => ({ node, cursor: node.id })),
  pageInfo: { hasNextPage: next !== null, endCursor: next },
});

type AuditArgs = { after?: string | null; filter?: { actionPrefix?: string | null } | null };

// 記録がないとき (デモは監査ログを残さない)。
export const Default: Story = { args: { overrides: { users } } };

export const Loading: Story = { args: { hold: holdAll } };

// 記録があり、続きもあるとき。ユーザー、システム、消したユーザーの操作と、送信元のない記録を並べる。
export const Events: Story = {
  args: { overrides: { users, auditEvents: () => page(events, 'AuditEvent:3') } },
};

// ユーザーの操作の詳細を開いたとき (字下げした JSON の要約と User-Agent)。
export const DetailsWithClient: Story = {
  args: Events.args,
  play: async ({ canvas }) => {
    const [details] = await canvas.findAllByRole('button', { name: '詳細' });
    if (details !== undefined) {
      await userEvent.click(details);
    }
  },
};

// システムの操作の詳細を開いたとき (JSON でない要約は、そのまま出す)。
export const DetailsSystem: Story = {
  args: Events.args,
  play: async ({ canvas }) => {
    const buttons = await canvas.findAllByRole('button', { name: '詳細' });
    const button = buttons[1];
    if (button !== undefined) {
      await userEvent.click(button);
    }
  },
};

// 消したユーザーの操作で、送信元の記録がないものの詳細を開いたとき (記録がないことの案内)。
export const DetailsWithoutClient: Story = {
  args: Events.args,
  play: async ({ canvas }) => {
    const buttons = await canvas.findAllByRole('button', { name: '詳細' });
    const button = buttons[2];
    if (button !== undefined) {
      await userEvent.click(button);
    }
  },
};

// 続きを読み込んで、最後まで出したとき (さらに読み込むのボタンが消える)。
export const LoadedMore: Story = {
  args: {
    overrides: {
      users,
      auditEvents: ({ after }: AuditArgs) =>
        after == null ? page(events, 'AuditEvent:3') : page(older, null),
    },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'さらに読み込む' }));
    await canvas.findByText('settings.update');
  },
};

// 続きを読み込めなかったとき。
export const MoreFailed: Story = {
  args: {
    overrides: {
      users,
      auditEvents: ({ after }: AuditArgs) => {
        if (after != null) {
          throw new Error('the demo server failed');
        }

        return page(events, 'AuditEvent:3');
      },
    },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'さらに読み込む' }));
  },
};

// 操作の種類で絞り込み、一致する記録がなかったとき。
export const FilteredEmpty: Story = {
  args: {
    overrides: {
      users,
      auditEvents: ({ filter }: AuditArgs) =>
        filter?.actionPrefix == null ? page(events, null) : page([], null),
    },
  },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByLabelText('操作の種類'), 'webhook.');
    await userEvent.click(await canvas.findByRole('button', { name: '絞り込む' }));
  },
};

// 管理者でないユーザーが開いたとき (サーバーが FORBIDDEN で拒否する)。
export const Forbidden: Story = {
  args: {
    overrides: merge(asViewer('MEMBER'), () => ({
      users,
      auditEvents: failingWith({ code: 'FORBIDDEN' }),
    })),
  },
};

// ログインしていないとき (tenban が 401 で拒否する)。管理の画面の枠は、ログインの画面へのリンクを出す。
export const SignedOut: Story = {
  args: {
    // 画面の問い合わせだけを拒否する。ヘッダーなどの問い合わせは、そのまま返す。
    hold: ({ name }) => {
      if (name === 'auditQuery') {
        throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
      }

      return false;
    },
  },
};

// 監査ログを読めなかったとき (FORBIDDEN 以外のエラー)。
export const Failed: Story = { args: { overrides: { users, auditEvents: failing() } } };
