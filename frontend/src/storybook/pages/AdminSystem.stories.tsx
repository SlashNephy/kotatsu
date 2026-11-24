import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { call, minutesAgo } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// システムの状態 (/admin/system) の画面 (#2056)。チューナーサーバーとチューナー、録画の失敗の累計、録画中の録画、
// チャンネルの TS の調査結果、インスタンスとリースの状態を並べる。
const meta = {
  title: 'ページ/管理/システム',
  component: PageStory,
  args: { url: '/admin/system' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Status = Record<string, unknown> & { tunerServer: Record<string, unknown> | null };

// healthy は、チューナーサーバーにつながり、時計の差が閾値の中に収まっている状態。
const healthy = {
  kind: 'MIRAKURUN',
  version: '4.0.0',
  degraded: false,
  degradedSince: null,
  lastError: '',
  clockSkewMs: 120,
  clockSkewed: false,
  clockSkewThresholdMs: 2000,
  checkedAt: minutesAgo(1),
  broadcastClockSkewMs: 300,
  broadcastClockSkewed: false,
  broadcastClockCheckedAt: minutesAgo(2),
};

// status は、ひと通りそろったシステムの状態。チューナーは空き、使用中、別の mikan のもの、故障を並べ、失敗の累計は
// 一度も失敗していない理由 (戻せない) と、失敗した理由と、0 に戻した理由を並べる。インスタンスとリースは、動いているものと止まったもの。
const status: Status = {
  instances: [
    {
      instanceId: 'instance-a',
      role: 'tenban',
      address: '192.0.2.10:8080',
      startedAt: minutesAgo(600),
      heartbeatAt: minutesAgo(0),
      alive: true,
    },
    {
      instanceId: 'instance-b',
      role: 'mikan',
      address: '192.0.2.11:8081',
      startedAt: minutesAgo(1200),
      heartbeatAt: minutesAgo(30),
      alive: false,
    },
  ],
  leases: [
    {
      name: 'scheduler',
      holderInstanceId: 'instance-a',
      generation: 12,
      acquiredAt: minutesAgo(600),
      expiresAt: minutesAgo(-1),
      valid: true,
    },
    {
      name: 'epg',
      holderInstanceId: 'instance-b',
      generation: 3,
      acquiredAt: minutesAgo(1200),
      expiresAt: minutesAgo(30),
      valid: false,
    },
  ],
  tunerServer: healthy,
  tuners: [
    { index: 0, name: 'Tuner 0', types: ['GR'], available: true, remote: false, fault: false },
    {
      index: 1,
      name: 'Tuner 1',
      types: ['BS', 'CS'],
      available: false,
      remote: true,
      fault: false,
    },
    { index: 2, name: 'Tuner 2', types: ['GR'], available: false, remote: false, fault: true },
  ],
  recordingFailures: [
    { reason: 'TUNER_UNAVAILABLE', count: 0, lastFailedAt: null, resetAt: null },
    { reason: 'STREAM_INTERRUPTED', count: 3, lastFailedAt: minutesAgo(180), resetAt: null },
    { reason: 'STORAGE_FULL', count: 0, lastFailedAt: null, resetAt: minutesAgo(1440) },
  ],
  pools: [],
};

// withStatus は、システムの状態を status に patch を重ねたものにする。
const withStatus = (patch: object = {}) => ({ systemStatus: () => ({ ...status, ...patch }) });

// withTunerServer は、チューナーサーバーの状態を、healthy に patch を重ねたものにする。
const withTunerServer = (patch: object) => withStatus({ tunerServer: { ...healthy, ...patch } });

// active は、録画中の録画。直近のビットレートがあり、ドロップのあるものと、まだ様子を書いていないものを並べる。
const active = {
  recordings: () => ({
    edges: [
      {
        cursor: '1',
        node: {
          id: 'Recording:active-1',
          name: '架空のドラマ　第３話',
          drops: 2,
          errors: 0,
          scrambled: 0,
          service: { id: 'Service:1024', name: '架空テレビ' },
          liveStats: { bitrate: 16_000_000, drops: 3, errors: 1, scrambled: 0 },
        },
      },
      {
        cursor: '2',
        node: {
          id: 'Recording:active-2',
          name: '架空のニュース',
          drops: 0,
          errors: 0,
          scrambled: 0,
          service: { id: 'Service:1025', name: '架空放送' },
          liveStats: null,
        },
      },
    ],
    pageInfo: { hasNextPage: false, endCursor: '2' },
  }),
};

// inspection は、チャンネルの TS を調査した結果。Kotatsu の知らないサービスと、コーデックの分からない ES と、ドロップを含む。
const inspection = {
  inspectChannel: () => ({
    transportStreamId: 0x7fe0,
    seconds: 5.04,
    totalBps: 16_800_000,
    programs: [
      {
        serviceId: 1024,
        name: '架空テレビ',
        bps: 15_000_000,
        streams: [
          {
            pid: 0x100,
            kind: 'VIDEO',
            streamType: 0x02,
            bps: 14_000_000,
            drops: 2,
            errors: 0,
            scrambled: 0,
          },
          {
            pid: 0x110,
            kind: 'AUDIO',
            streamType: 0x0f,
            bps: 256_000,
            drops: 0,
            errors: 0,
            scrambled: 0,
          },
        ],
      },
      {
        serviceId: 1025,
        name: null,
        bps: 1_000_000,
        streams: [
          {
            pid: 0x200,
            kind: 'UNKNOWN',
            streamType: null,
            bps: 1_000_000,
            drops: 0,
            errors: 0,
            scrambled: 12,
          },
        ],
      },
    ],
    others: [
      { pid: 0x0, kind: 'PSI', bps: 20_000 },
      { pid: 0x12, kind: 'PSI', bps: 300_000 },
    ],
  }),
};

// lineup は、チャンネルの変更の知らせ。まだ読んでいない知らせを含むので、開くと既読にする。
// ヘッダーのお知らせ (unreadOnly) には、まだ読んでいない知らせだけを返す。
const lineupEdges = [
  {
    cursor: '1',
    node: {
      __typename: 'Notification',
      id: 'Notification:lineup-1',
      kind: 'LINEUP_CHANGED',
      message:
        'BS の局の顔ぶれが変わりました。架空放送（サービス 999）がチューナーサーバーにありません',
      occurredAt: minutesAgo(60),
      readAt: null,
    },
  },
  {
    cursor: '2',
    node: {
      __typename: 'Notification',
      id: 'Notification:lineup-2',
      kind: 'LINEUP_CHANGED',
      message: 'BS15 の選局が続けて失敗しています',
      occurredAt: minutesAgo(1440),
      readAt: minutesAgo(1000),
    },
  },
];

const lineup = {
  notifications: ({ unreadOnly }: { unreadOnly?: boolean | null }) => ({
    edges: lineupEdges.filter((edge) => unreadOnly !== true || edge.node.readAt === null),
    pageInfo: { hasNextPage: false, endCursor: '2' },
  }),
};

// release は、新しいバージョンが出ている状態。
const release = (root: DemoRoot) => ({
  version: () => ({
    ...call<object>(root, 'version'),
    version: '1.4.0',
    latestRelease: {
      version: '1.5.0',
      url: 'https://example.com/releases/1.5.0',
      publishedAt: minutesAgo(2880),
    },
  }),
});

// ひと通りそろった状態。
export const Default: Story = { args: { overrides: { ...withStatus(), ...active } } };

export const Loading: Story = { args: { hold: holdAll } };

// 管理者でないとき (サーバーが FORBIDDEN で拒んだ)。
export const Forbidden: Story = {
  args: { overrides: { systemStatus: failingWith({ code: 'FORBIDDEN' }) } },
};

// 状態を読めなかったとき。
export const Failed: Story = { args: { overrides: { systemStatus: failing() } } };

// 起動した直後で、チューナーサーバーをまだ確認しておらず、チューナーも録画中の録画もないとき。
export const Unchecked: Story = {
  args: {
    overrides: {
      ...withStatus({ tunerServer: null, tuners: [], recordingFailures: [] }),
      recordings: () => ({ edges: [], pageInfo: { hasNextPage: false, endCursor: null } }),
    },
  },
};

// 新しいバージョンが出ていて、チャンネルの変更の通知があるとき。
export const Notices: Story = {
  args: {
    overrides: (root: DemoRoot) => ({ ...withStatus(), ...active, ...release(root), ...lineup }),
  },
};

// チャンネルの変更の知らせを既読にできなかったとき。
export const LineupMarkFailed: Story = {
  args: {
    overrides: { ...withStatus(), ...lineup, markNotificationsRead: failing() },
  },
};

// チューナーサーバーにつながらない縮退中で、最後の失敗の理由を出すとき。種類は分かるが、バージョンは分からない。
export const Degraded: Story = {
  args: {
    overrides: withTunerServer({
      degraded: true,
      degradedSince: minutesAgo(15),
      lastError: 'dial tcp 192.0.2.20:40772: connect: connection refused',
      version: null,
    }),
  },
};

// チューナーサーバーの種類がまだ分からず、時計の差も測れていないとき。
export const KindUnknown: Story = {
  args: {
    overrides: withTunerServer({
      kind: null,
      version: null,
      clockSkewMs: null,
      broadcastClockSkewMs: null,
      broadcastClockCheckedAt: null,
    }),
  },
};

// EDCB で、時計の差を測れないとき。
export const EdcbClockUnavailable: Story = {
  args: {
    overrides: withTunerServer({ kind: 'EDCB', version: null, clockSkewMs: null }),
  },
};

// 時計の差と、放送の時刻との差が、どちらも閾値を超えたとき。
export const ClockSkewed: Story = {
  args: {
    overrides: withTunerServer({
      clockSkewMs: 4500,
      clockSkewed: true,
      broadcastClockSkewMs: -3200,
      broadcastClockSkewed: true,
    }),
  },
};

// 失敗の累計を 0 に戻す前に確認するとき。
export const ConfirmResetFailures: Story = {
  args: { overrides: withStatus() },
  play: async ({ canvas }) => {
    const buttons = await canvas.findAllByRole('button', { name: '0 に戻す' });
    const enabled = buttons.find((button) => !button.hasAttribute('disabled'));
    if (enabled === undefined) {
      throw new Error('the story could not find enabled');
    }
    await userEvent.click(enabled);
    await within(document.body).findByRole('dialog');
  },
};

// 失敗の累計を 0 に戻せなかったとき。
export const ResetFailuresFailed: Story = {
  args: { overrides: { ...withStatus(), resetRecordingFailureCounts: failing() } },
  play: async (context) => {
    await ConfirmResetFailures.play?.(context);
    const dialog = await within(document.body).findByRole('dialog');
    await userEvent.click(within(dialog).getByRole('button', { name: '0 に戻す' }));
  },
};

// チャンネルの TS を調査した結果を出したとき。
export const Inspected: Story = {
  args: { overrides: { ...withStatus(), ...inspection } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '調査する' }));
  },
};

// 調査したが、PAT を受け取れず TSID が分からなかったとき。
export const InspectedWithoutPat: Story = {
  args: {
    overrides: {
      ...withStatus(),
      inspectChannel: () => ({ ...inspection.inspectChannel(), transportStreamId: null }),
    },
  },
  play: Inspected.play,
};

// チャンネルの TS を調査できなかったとき。
export const InspectFailed: Story = {
  args: { overrides: { ...withStatus(), inspectChannel: failing() } },
  play: Inspected.play,
};

// 調査しているあいだ (ボタンが読み込み中)。
export const Inspecting: Story = {
  args: {
    overrides: withStatus(),
    hold: (operation) => operation.name === 'ChannelInspectorMutation',
  },
  play: Inspected.play,
};

// サービスがなく、調査するチャンネルを選べないとき。
export const NoChannels: Story = {
  args: { overrides: { ...withStatus(), services: () => [] } },
};
