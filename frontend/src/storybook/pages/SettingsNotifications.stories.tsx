import type { Meta, StoryObj } from '@storybook/react-vite';

import { holdAll, PageStory } from '../PageStory';
import { asViewer, merge, removeProperty, replaceProperty } from '../conditions';

// 通知の設定 (/settings/notifications) の画面 (#2056)。この端末の Web Push の購読の有無、Web Push を使えない端末、
// 管理者にだけ届く種類の出し分けを並べる。
const meta = {
  title: 'ページ/設定/通知',
  component: PageStory,
  args: { url: '/settings/notifications' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const kinds = [
  'RECORDING_STARTED',
  'RECORDING_COMPLETED',
  'RECORDING_FAILED',
  'RESERVATION_CONFLICT',
  'STORAGE_LOW',
  'TUNER_SERVER_UNREACHABLE',
  'WATCH_REMINDER',
  'ANIME_RECORDED',
  'ANIME_RECORD_FAILED',
  'DRAMA_RECORDED',
  'DRAMA_RECORD_FAILED',
  'SCRAMBLED_RECORDINGS',
  'LINEUP_CHANGED',
];

const endpoint = 'https://push.example.com/kotatsu-demo';

const preferences = (subscribed: boolean) => ({
  webPushPublicKey: () => 'BKotatsuDemoPublicKey',
  webPushSubscriptions: () =>
    subscribed
      ? [
          {
            id: 'WebPushSubscription:1',
            endpoint,
            userAgent: navigator.userAgent,
            createdAt: '2026-11-01T12:00:00+09:00',
          },
        ]
      : [],
  notificationPreferences: () =>
    kinds.map((kind, index) => ({
      kind,
      inApp: true,
      webPush: index % 3 === 0,
      ruleReservations: kind === 'RECORDING_STARTED',
    })),
});

// fakeServiceWorker は、Storybook に Service Worker がないので、この端末の購読を返す Service Worker の代わりを置く。
const fakeServiceWorker = (subscription: { endpoint: string } | null) => () =>
  replaceProperty(navigator, 'serviceWorker', {
    ready: Promise.resolve({
      pushManager: { getSubscription: () => Promise.resolve(subscription) },
    }),
  });

// この端末で購読しているとき。
export const Default: Story = {
  args: { overrides: preferences(true) },
  beforeEach: fakeServiceWorker({ endpoint }),
};

export const Loading: Story = { args: { hold: holdAll } };

// この端末では購読していないとき。
export const NotSubscribed: Story = {
  args: { overrides: preferences(false) },
  beforeEach: fakeServiceWorker(null),
};

// この端末の購読を確認している途中 (切り替えを押せない)。
export const CheckingSubscription: Story = {
  args: { overrides: preferences(false) },
  beforeEach: () =>
    replaceProperty(navigator, 'serviceWorker', { ready: new Promise(() => undefined) }),
};

// Web Push を使えないブラウザのとき。
export const PushUnsupported: Story = {
  args: { overrides: preferences(false) },
  beforeEach: () => removeProperty(window, 'PushManager'),
};

// メンバーから見たとき。管理者にだけ届く種類 (スクランブルの警告など) を出さない。
export const AsMember: Story = {
  args: { overrides: merge(() => preferences(true), asViewer('MEMBER')) },
  beforeEach: fakeServiceWorker({ endpoint }),
};
