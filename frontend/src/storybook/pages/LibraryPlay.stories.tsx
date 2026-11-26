import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { demoStreamURL } from '../../demo/streamURL';
import { GraphQLRequestError } from '../../relay/requestError';
import { asViewer, merge, minutesAgo, withStorage } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// ライブラリのファイルの再生 (/library/play) の画面 (#2056)。静的なデモと同じ Service Worker で、デモの映像を流す。
// 番組の分かるファイルと分からないファイル、インデックスを作っている途中と書いている途中、続きからの再生、CM の区間、
// 字幕の自動生成の状態、ファイルがない・読めない・ほかで使っているとき、ログインしていないとき、再生の経路を並べる。
const path = 'バラエティ/こたつの時間.m2ts';

const meta = {
  title: 'ページ/ライブラリ/再生',
  component: PageStory,
  args: { url: `/library/play?library=archive&path=${encodeURIComponent(path)}` },
  parameters: { layout: 'fullscreen' },
  // 音を出さない。
  beforeEach: () =>
    withStorage({ 'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }) }),
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const recordedFrom = minutesAgo(120);
const recordedUntil = minutesAgo(90);

// broadcast は、TS から分かった放送 (サービスと番組)。
const broadcast = {
  serviceName: 'こたつテレビ',
  jikkyoChannelId: null,
  recordedFrom,
  recordedUntil,
  service: { id: 'Service:1024', name: 'こたつテレビ', logoUrl: null },
  program: {
    name: '[字]こたつの時間',
    description: 'こたつで過ごす冬の夜を、家族の会話とともに描きます。',
    extended: [],
    startAt: recordedFrom,
    endAt: recordedUntil,
  },
};

// file は、ライブラリにある 30 分の TS。patch で差し替える。
const file = (patch: Record<string, unknown> = {}) => ({
  libraryFile: () => ({
    id: 'LibraryFile:archive/こたつの時間',
    fileId: 'archive-1',
    name: 'こたつの時間.m2ts',
    size: 2_400_000_000,
    modifiedAt: recordedUntil,
    videoFormat: 'HD',
    watchPositionMs: null,
    streamUrl: demoStreamURL,
    hlsUrl: '/demo/library/playlist.m3u8',
    library: { name: 'アーカイブ', kind: 'FOLDER' },
    // 実況のチャンネルのない局なので、過去ログは読まない。
    jikkyoCommentsUrl: '/demo/library/jikkyo.json',
    thumbnailUrl: null,
    previewUrl: null,
    cmSections: [],
    autoCaptions: [],
    autoCaptionsStatus: {
      state: 'NOT_GENERATED',
      generatedAt: null,
      model: null,
      count: 0,
      progress: null,
    },
    broadcast,
    audios: [],
    quality: { drops: 0, errors: 0, scrambled: 0 },
    index: { complete: true, writing: false, seekableSeconds: 1800, durationSeconds: 1800 },
    ...patch,
  }),
});

const captionsStatus = (patch: Record<string, unknown>) => ({
  state: 'NOT_GENERATED',
  generatedAt: null,
  model: null,
  count: 0,
  progress: null,
  ...patch,
});

export const Default: Story = { args: { overrides: file() } };

export const Loading: Story = { args: { hold: holdAll } };

// TS から放送が分からないファイル (番組の名前の代わりにファイル名を出し、字幕の自動生成を出さない)。
export const WithoutBroadcast: Story = { args: { overrides: file({ broadcast: null }) } };

// Google Drive のファイル (パスでなくファイル名を出す)。
export const GoogleDrive: Story = {
  args: { overrides: file({ library: { name: 'マイドライブ', kind: 'GOOGLE_DRIVE' } }) },
};

// シークのインデックスを作っている途中 (シークできる所までのマークと、作っている通知)。
export const Indexing: Story = {
  args: {
    overrides: file({
      index: { complete: false, writing: false, seekableSeconds: 600, durationSeconds: null },
    }),
  },
};

// ほかのプログラムが書いている途中のファイル。
export const Writing: Story = {
  args: {
    overrides: file({
      index: { complete: false, writing: true, seekableSeconds: 600, durationSeconds: null },
    }),
  },
};

// 前回の続き (5 分) から再生するとき。
export const Resumed: Story = { args: { overrides: file({ watchPositionMs: 5 * 60_000 }) } };

// URL の位置 (2 分) から再生するとき。
export const StartAt: Story = {
  args: {
    overrides: file(),
    url: `/library/play?library=archive&path=${encodeURIComponent(path)}&t=120`,
  },
};

// 再生の始まりが CM の区間のとき。CM を飛ばすボタンを出す。
export const InCm: Story = {
  args: { overrides: file({ cmSections: [{ startSeconds: 0, endSeconds: 60 }] }) },
};

// 字幕の自動生成の状態。
export const CaptionsQueued: Story = {
  args: { overrides: file({ autoCaptionsStatus: captionsStatus({ state: 'QUEUED' }) }) },
};

export const CaptionsRunning: Story = {
  args: {
    overrides: file({
      autoCaptionsStatus: captionsStatus({
        state: 'RUNNING',
        progress: { ratio: 0.42, remainingSeconds: 180 },
      }),
    }),
  },
};

export const CaptionsGenerated: Story = {
  args: {
    overrides: file({
      autoCaptions: [
        { startSeconds: 0, endSeconds: 4, text: 'こたつ山のふもとから、けさの様子をお伝えします' },
      ],
      autoCaptionsStatus: captionsStatus({
        state: 'GENERATED',
        count: 1,
        generatedAt: minutesAgo(30),
        model: 'whisper-small',
      }),
    }),
  },
};

export const CaptionsFailed: Story = {
  args: { overrides: file({ autoCaptionsStatus: captionsStatus({ state: 'FAILED' }) }) },
};

// モデルがなくて取りやめたとき。管理者には、モデルを置く画面へのリンクを出す。
export const CaptionsNotConfigured: Story = {
  args: { overrides: file({ autoCaptionsStatus: captionsStatus({ state: 'NOT_CONFIGURED' }) }) },
};

// 視聴だけのユーザーには、字幕を生成するボタンを出さない。
export const AsViewer: Story = {
  args: {
    overrides: merge(
      () => file({ autoCaptionsStatus: captionsStatus({ state: 'NOT_CONFIGURED' }) }),
      asViewer('VIEWER'),
    ),
  },
};

// 字幕の生成を頼めなかったとき。
export const GenerateCaptionsFailed: Story = {
  args: { overrides: { ...file(), generateLibraryCaptions: failing() } },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: '字幕を生成する' }));
  },
};

// ファイルがないとき、対応していないファイル、ほかで使っているとき、読み込めないとき。
export const FileNotFound: Story = {
  args: { overrides: { libraryFile: failingWith({ code: 'NOT_FOUND' }) } },
};

export const Unsupported: Story = {
  args: { overrides: { libraryFile: failingWith({ code: 'BAD_USER_INPUT' }) } },
};

export const Busy: Story = {
  args: { overrides: { libraryFile: failingWith({ code: 'CONFLICT' }) } },
};

export const Error: Story = { args: { overrides: { libraryFile: failing() } } };

// ログインしていないとき (共有された URL を開いた)。
export const SignedOut: Story = {
  args: {
    hold: (operation: { name: string }) => {
      if (operation.name === 'playLibraryFileQuery') {
        throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
      }

      return false;
    },
  },
};

// 映像を読めないとき。
export const PlaybackError: Story = {
  args: { overrides: file({ streamUrl: '/demo/missing.m2ts' }) },
};

// kyusu がいて、変換した HLS で再生するとき (Storybook には kyusu がないので、映像は届かない)。
export const KyusuRoute: Story = {
  args: {
    overrides: {
      ...file(),
      transcoding: () => ({
        available: true,
        presets: [{ name: '720p', codec: 'H264', passthrough: false, videoBitrate: 3_000_000 }],
      }),
    },
  },
  beforeEach: () =>
    withStorage({
      'kotatsu.player.volume': JSON.stringify({ volume: 0, muted: true }),
      'kotatsu.player.quality': '720p',
    }),
};

// BS4K のファイルで、kyusu がいないとき (ブラウザの中では変換できない)。
export const UnavailableUHD: Story = { args: { overrides: file({ videoFormat: 'UHD' }) } };
