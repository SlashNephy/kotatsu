import type { Meta, StoryObj } from '@storybook/react-vite';

import { asViewer, merge, minutesAgo, recordingsWith } from '../conditions';
import { firstRecordingPath, holdAll, PageStory } from '../PageStory';

// 録画の詳細 (/recordings/\$recordingId) の画面 (#2056)。
const meta = {
  title: 'ページ/録画/詳細',
  component: PageStory,
  args: { url: firstRecordingPath('/recordings/:id') },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 録画が見つからないとき (消した録画の URL を開いたなど)。
export const NotFound: Story = {
  args: { url: () => Promise.resolve('/recordings/Recording:missing') },
};

// 録画の状態ごとの表示。いちばん新しい録画を、それぞれの状態にして開く。
// 実際の放送の時刻 (TS の EIT[p/f]) も、予定どおりに放送したことにする。
const ended = () => ({
  startAt: minutesAgo(90),
  endAt: minutesAgo(30),
  recordedFrom: minutesAgo(90),
  recordedUntil: minutesAgo(30),
  broadcastStartedAt: minutesAgo(90),
  broadcastEndedAt: minutesAgo(30),
  liveStats: null,
});

const stream = (drops: number, errors: number, scrambled = 0) => [
  {
    pid: 0x100,
    kind: 'VIDEO',
    packets: 1_000_000,
    drops,
    errors,
    scrambled,
    streamType: 2,
    componentTag: 0,
  },
  {
    pid: 0x110,
    kind: 'AUDIO',
    packets: 100_000,
    drops: 0,
    errors: 0,
    scrambled: 0,
    streamType: 15,
    componentTag: 16,
  },
];

// 録画中。録りながら、受け取った量と速さを出す。
export const Recording: Story = {
  args: {
    overrides: recordingsWith({
      state: 'RECORDING',
      recordedUntil: null,
      liveStats: {
        bitrate: 16_000_000,
        bytes: 3_000_000_000,
        drops: 0,
        errors: 0,
        scrambled: 0,
        updatedAt: new Date().toISOString(),
      },
    }),
  },
};

export const Completed: Story = {
  args: { overrides: recordingsWith({ ...ended(), state: 'COMPLETED' }) },
};

// 録れたが、ドロップとエラーがあり、途中に欠けた区間があるとき。
export const CompletedWithErrors: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED_WITH_ERRORS',
      drops: 128,
      errors: 12,
      streamQuality: stream(128, 12),
      gaps: [{ from: minutesAgo(70), to: minutesAgo(68) }],
    }),
  },
};

// 途中から、または途中までしか録れなかったとき。
export const Partial: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'PARTIAL',
      recordedFrom: minutesAgo(70),
      recordedUntil: minutesAgo(45),
    }),
  },
};

// 失敗した理由ごとの表示。
const failureReasons = [
  'TUNER_UNAVAILABLE',
  'TUNER_SERVER_UNREACHABLE',
  'STREAM_INTERRUPTED',
  'STORAGE_UNAVAILABLE',
  'STORAGE_FULL',
  'PROGRAM_NOT_FOUND',
  'CANCELLED',
  'RECORDER_UNAVAILABLE',
  'TUNER_PREEMPTED',
  'TUNING_REFUSED',
] as const;

const failed = (
  failureReason: (typeof failureReasons)[number],
  failureDetail: string | null = null,
): Story => ({
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'FAILED',
      recordedUntil: null,
      // 録れなかったので、実際の放送の時刻も分からない。
      broadcastStartedAt: null,
      broadcastEndedAt: null,
      failureReason,
      failureDetail,
      thumbnailUrl: null,
      previewUrl: null,
    }),
  },
});

export const FailedTunerUnavailable = failed('TUNER_UNAVAILABLE');
export const FailedTunerServerUnreachable = failed(
  'TUNER_SERVER_UNREACHABLE',
  'dial tcp 192.0.2.1:40772: connect: connection refused',
);
export const FailedStreamInterrupted = failed('STREAM_INTERRUPTED');
export const FailedStorageUnavailable = failed('STORAGE_UNAVAILABLE');
export const FailedStorageFull = failed('STORAGE_FULL');
export const FailedProgramNotFound = failed('PROGRAM_NOT_FOUND');
export const FailedCancelled = failed('CANCELLED');
export const FailedRecorderUnavailable = failed('RECORDER_UNAVAILABLE');
export const FailedTunerPreempted = failed('TUNER_PREEMPTED');
export const FailedTuningRefused = failed('TUNING_REFUSED');

// スクランブルされたまま録れた (B-CAS カードが読めていないかもしれない) とき。
export const ScrambleWarning: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED_WITH_ERRORS',
      scrambled: 900_000,
      scrambleWarning: true,
      scrambledRatio: 0.9,
      streamQuality: stream(0, 0, 900_000),
    }),
  },
};

// 消されないよう保護した録画。
export const Protected: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED',
      protected: true,
    }),
  },
};

// 録画の途中で、番組が番組表から消えたとき。
export const ProgramLost: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED',
      programLost: true,
    }),
  },
};

// 放送されなかった (予定の時刻に放送が始まらなかった) とき。
export const NotBroadcast: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED',
      notBroadcast: true,
    }),
  },
};

// 放送の始まりと終わりが、予定からずれたとき。
export const BroadcastMoved: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED',
      broadcastStartedAt: minutesAgo(85),
      broadcastEndedAt: minutesAgo(25),
    }),
  },
};

// 番組の概要がなく、もとのルールが消されたとき。
export const NoDescriptionDeletedRule: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED',
      description: '',
      rule: null,
      ruleName: '消したルール',
    }),
  },
};

// CM の区間の見分けの状態ごとの表示。
const progress = { ratio: 0.4, remainingSeconds: 90, updatedAt: new Date().toISOString() };
const cm = (state: string, extra: object = {}): Story => ({
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED',
      cmDetection: {
        state,
        detectedAt: null,
        progress: null,
        note: null,
        videoMissingSeconds: 0,
        ...extra,
      },
      cmSections:
        state === 'DETECTED'
          ? [
              { from: minutesAgo(80), to: minutesAgo(78) },
              { from: minutesAgo(60), to: minutesAgo(57) },
            ]
          : [],
    }),
  },
});

export const CmQueued = cm('QUEUED');
export const CmRunning = cm('RUNNING', { progress });
export const CmDetected = cm('DETECTED', { detectedAt: minutesAgo(10) });
export const CmNotDetected = cm('NOT_DETECTED');
export const CmFailed = cm('FAILED', { note: 'the video could not be decoded' });
export const CmVideoMissing = cm('DETECTED', {
  detectedAt: minutesAgo(10),
  videoMissingSeconds: 42,
  note: '映像のない区間がありました',
});

// 自動字幕の状態ごとの表示。
const captions = (state: string, extra: object = {}): Story => ({
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED',
      autoCaptionsStatus: {
        state,
        generatedAt: null,
        model: null,
        count: 0,
        progress: null,
        ...extra,
      },
      autoCaptions:
        state === 'GENERATED'
          ? [{ from: minutesAgo(89), to: minutesAgo(88.9), text: 'こんばんは、こたつの時間です' }]
          : [],
    }),
  },
});

export const CaptionsNotConfigured = captions('NOT_CONFIGURED');
export const CaptionsNotGenerated = captions('NOT_GENERATED');
export const CaptionsQueued = captions('QUEUED');
export const CaptionsRunning = captions('RUNNING', { progress });
export const CaptionsGenerated = captions('GENERATED', {
  generatedAt: minutesAgo(5),
  model: 'demo',
  count: 1,
});
export const CaptionsFailed = captions('FAILED');

// サムネイルとプレビューがまだないとき。
export const NoThumbnail: Story = {
  args: {
    overrides: recordingsWith({
      ...ended(),
      state: 'COMPLETED',
      thumbnailUrl: null,
      previewUrl: null,
      watchFrame: null,
    }),
  },
};

// 視聴者 (録画を消したり、タグを付けたりできない) から見たとき。
export const AsViewer: Story = {
  args: {
    overrides: merge(recordingsWith({ ...ended(), state: 'COMPLETED' }), asViewer('VIEWER')),
  },
};

// メンバーから見たとき。字幕のモデルが置かれていない録画で、モデルを置く画面へのリンク (管理者だけ) を出さない。
export const AsMember: Story = {
  args: {
    overrides: merge(
      recordingsWith({
        ...ended(),
        state: 'COMPLETED',
        autoCaptionsStatus: {
          state: 'NOT_CONFIGURED',
          generatedAt: null,
          model: null,
          count: 0,
          progress: null,
        },
      }),
      asViewer('MEMBER'),
    ),
  },
};
