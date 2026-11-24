import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// ユーザーの管理 (/admin/users) の画面 (#2056)。ユーザーの行の状態と操作の途中、ユーザーを作るフォーム、
// 開いたときと操作のときの本人確認を並べる。
const meta = {
  title: 'ページ/管理/ユーザー',
  component: PageStory,
  args: { url: '/admin/users' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Context = Parameters<NonNullable<Story['play']>>[0];

// users は、自分 (管理者) と、メンバーと、無効にした視聴者。
const users = {
  users: () => [
    {
      id: 'User:demo',
      username: 'demo',
      displayName: 'デモ',
      role: 'ADMIN',
      disabled: false,
      createdAt: '2026-10-01T00:00:00Z',
    },
    {
      id: 'User:member',
      username: 'member',
      displayName: '家族のメンバー',
      role: 'MEMBER',
      disabled: false,
      createdAt: '2026-10-15T00:00:00Z',
    },
    {
      id: 'User:viewer',
      username: 'viewer',
      displayName: '見るだけの人',
      role: 'VIEWER',
      disabled: true,
      createdAt: '2026-11-01T00:00:00Z',
    },
  ],
  disableUser: ({ id }: { id: string }) => ({ id, disabled: true }),
  createUser: ({ input }: { input: { username: string } }) => ({
    id: 'User:new',
    username: input.username,
  }),
};

// row は、ユーザー名が username の行。
async function row({ canvas, canvasElement }: Context, username: string) {
  await canvas.findByText(`@${username}`);
  const element = canvasElement.querySelector<HTMLElement>(`[data-user="${username}"]`);
  if (element === null) {
    throw new Error(`no row for ${username}`);
  }

  return within(element);
}

// clickInRow は、username の行の name のボタンを押す。
async function clickInRow(context: Context, username: string, name: string) {
  const target = await row(context, username);
  await userEvent.click(target.getByRole('button', { name }));
}

// 自分は無効にも削除にもできず、無効にしたユーザーには有効に戻すボタンを出す。
export const Default: Story = { args: { overrides: users } };

export const Loading: Story = { args: { hold: holdAll } };

// 管理者でないとき (サーバーが FORBIDDEN で拒んだ)。
export const Forbidden: Story = {
  args: { overrides: { users: failingWith({ code: 'FORBIDDEN' }) } },
};

// ユーザーを読めなかったとき。
export const Failed: Story = { args: { overrides: { users: failing() } } };

// 表示名とロールを変えているとき。
export const Editing: Story = {
  args: { overrides: users },
  play: async (context) => clickInRow(context, 'member', '編集'),
};

// パスワードを設定し直しているとき (強さを出す)。
export const ResettingPassword: Story = {
  args: { overrides: users },
  play: async (context) => {
    await clickInRow(context, 'member', 'パスワードを設定し直す');
    const target = await row(context, 'member');
    await userEvent.type(target.getByLabelText('新しいパスワード'), 'kotatsu-demo');
  },
};

// 削除を確認しているとき (ユーザー名を打つまで、削除のボタンを押せない)。
export const Deleting: Story = {
  args: { overrides: users },
  play: async (context) => clickInRow(context, 'member', '削除'),
};

// 削除の確認で、ユーザー名を打ち、削除のボタンを押せるようになったとき。
export const DeleteConfirmed: Story = {
  args: { overrides: users },
  play: async (context) => {
    await clickInRow(context, 'member', '削除');
    const target = await row(context, 'member');
    await userEvent.type(target.getByRole('textbox'), 'member');
  },
};

// ユーザーを無効にし、保存した知らせを出したとき。
export const Disabled: Story = {
  args: { overrides: users },
  play: async (context) => clickInRow(context, 'member', '無効にする'),
};

// 操作に失敗したとき (失敗の知らせを出す)。
export const OperationFailed: Story = {
  args: { overrides: { ...users, disableUser: failing() } },
  play: Disabled.play,
};

// 操作の途中で、直近の認証の期限が切れ、本人確認を求めたとき。
export const ReauthDuringOperation: Story = {
  args: {
    overrides: { ...users, disableUser: failingWith({ code: 'REAUTHENTICATION_REQUIRED' }) },
  },
  play: async (context) => {
    await Disabled.play?.(context);
    await within(document.body).findByRole('dialog');
  },
};

// ユーザーを作るフォームに、パスワードまで入れたとき (強さを出す)。
export const CreatingWithPassword: Story = {
  args: { overrides: users },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByRole('textbox', { name: 'ユーザー名' }), 'guest');
    await userEvent.type(canvas.getByLabelText('パスワード'), 'correct-horse-battery');
  },
};

// ユーザーを作り、作った知らせを出したとき。
export const Created: Story = {
  args: { overrides: users },
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByRole('textbox', { name: 'ユーザー名' }), 'guest');
    await userEvent.click(canvas.getByRole('button', { name: 'ユーザーを作成' }));
  },
};

// 開いたときに、しばらく認証していないので、内容を出す前に本人確認を求めるとき (パスワードだけ)。
export const ReauthRequired: Story = {
  args: { overrides: users, auth: { '/internal/auth/reauth': { recent: false } } },
};

// 本人確認を求めるときに、認証アプリ、リカバリーコード、パスキー、IdP のどれでも確認できるとき。
export const ReauthRequiredAllMethods: Story = {
  args: {
    overrides: {
      ...users,
      secondFactors: () => ({ totp: true, remainingRecoveryCodes: 8 }),
      passkeys: () => [{ id: 'Passkey:1' }],
    },
    auth: {
      '/internal/auth/reauth': { recent: false },
      '/internal/auth/oidc': { enabled: true, displayName: '架空の IdP', linked: true },
    },
  },
};

// IdP での本人確認に失敗して戻ってきたとき (理由を出す)。戻り先の URL の error を、Story の間だけ足す。
export const IdpReauthFailed: Story = {
  args: { overrides: users },
  beforeEach: () => {
    const original = window.location.href;
    const url = new URL(original);
    url.searchParams.set('error', 'access_denied');
    window.history.replaceState(window.history.state, '', url);

    return () => window.history.replaceState(window.history.state, '', original);
  },
};
