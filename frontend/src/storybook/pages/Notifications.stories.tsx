import type { Meta, StoryObj } from '@storybook/react-vite';

import { paginate } from '../../demo/data';
import { GraphQLRequestError } from '../../relay/requestError';
import { failing, holdAll, PageStory } from '../PageStory';

// 通知の一覧 (/notifications) の画面 (#2056)。新着と既読の通知、通知が示す画面や外部のページへのリンク、既読にできなかったとき、
// 読み込めなかったときの表示を並べる。デモには通知がないので、既定は空の一覧になる。
const meta = {
  title: 'ページ/通知',
  component: PageStory,
  args: { url: '/notifications' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const notifications = [
  // 未読。番組名を、ほかの画面と同じくマーク付きで描く。録画の詳細へのリンク。
  {
    id: 'Notification:1',
    kind: 'RECORDING_COMPLETED',
    occurredAt: '2026-11-21T22:00:00+09:00',
    message: '「[新]ドラマ「冬の灯」#1」の録画が終わりました',
    payload: JSON.stringify({ name: '[新]ドラマ「冬の灯」#1' }),
    url: '/recordings/Recording:1',
    externalUrl: null,
    readAt: null,
  },
  // 未読。視聴予約の通知は、ライブ視聴へのリンクにする。
  {
    id: 'Notification:2',
    kind: 'WATCH_REMINDER',
    occurredAt: '2026-11-21T20:55:00+09:00',
    message: 'まもなく「夜のニュース」が始まります',
    payload: JSON.stringify({ name: '夜のニュース' }),
    url: '/live/Service:1024',
    externalUrl: null,
    readAt: null,
  },
  // 既読。見たアニメを記録した通知は、記録したページへのリンクも出す。
  {
    id: 'Notification:3',
    kind: 'ANIME_RECORDED',
    occurredAt: '2026-11-20T23:30:00+09:00',
    message: 'アニメ「星の旅人」第3話を記録しました',
    payload: '{}',
    url: null,
    externalUrl: 'https://anime.example.com/works/1/episodes/3',
    readAt: '2026-11-21T08:00:00+09:00',
  },
  // 既読。示す画面がなく、外部のページが https でない (リンクを出さない)。
  {
    id: 'Notification:4',
    kind: 'STORAGE_LOW',
    occurredAt: '2026-11-19T03:00:00+09:00',
    message: '保存先の空きが 10% を切りました',
    payload: '{}',
    url: 'https://other.example.com/',
    externalUrl: 'http://insecure.example.com/',
    readAt: '2026-11-19T09:00:00+09:00',
  },
];

const list = (args: { first: number; after?: string | null }) =>
  paginate(notifications, args.first, args.after);

// signedOut は、通知の一覧の読み込みを、ログインしていないときの拒否 (HTTP 401) で失敗させる。
const signedOut = (operation: { name: string }) => {
  if (operation.name === 'NotificationListQuery') {
    throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
  }

  return false;
};

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 新着 (未読) と既読の通知があるとき。開くと、新着を既読にする。
export const Notifications: Story = {
  args: {
    overrides: {
      notifications: list,
      unreadNotificationCount: () => 2,
      markAllNotificationsRead: () => 2,
    },
  },
};

// 新着を既読にできなかったとき。
export const MarkFailed: Story = {
  args: {
    overrides: {
      notifications: list,
      unreadNotificationCount: () => 2,
      markAllNotificationsRead: failing(),
    },
  },
};

// ログインしていないとき (ログインへのリンク)。
export const SignedOut: Story = { args: { hold: signedOut } };

// 読み込めなかったとき。
export const Error: Story = { args: { overrides: { notifications: failing() } } };
