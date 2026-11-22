import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// 保存先 (/admin/storage) の画面 (#2056)。保存先プールの容量と、録画とファイルの片付け (ファイルの見つからない録画、
// DB にないファイル、ライブラリの生成物、ファイル名の付け直し) の、探した結果と操作の途中を並べる。
const meta = {
  title: 'ページ/管理/保存先',
  component: PageStory,
  args: { url: '/admin/storage' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Canvas = Parameters<NonNullable<Story['play']>>[0]['canvas'];

const GB = 1e9;
const at = (day: number) => `2026-11-${String(day).padStart(2, '0')}T21:00:00+09:00`;

// pools は、使い方の違う保存先プール。閾値に余裕のあるもの、9 割を超えたもの (閾値なし)、空きが閾値を下回ったもの、
// 空き容量を測れないものを並べる。
const pools = {
  systemStatus: () => ({
    pools: [
      {
        id: 'default',
        path: '/mnt/recordings',
        priority: 10,
        minFreeBytes: 100 * GB,
        freeBytes: 400 * GB,
        totalBytes: 1000 * GB,
      },
      {
        id: 'archive',
        path: '/mnt/archive',
        priority: 0,
        minFreeBytes: 0,
        freeBytes: 50 * GB,
        totalBytes: 1000 * GB,
      },
      {
        id: 'full',
        path: '/mnt/full',
        priority: -10,
        minFreeBytes: 50 * GB,
        freeBytes: 20 * GB,
        totalBytes: 500 * GB,
      },
      {
        id: 'remote',
        path: '/mnt/remote',
        priority: 0,
        minFreeBytes: 0,
        freeBytes: null,
        totalBytes: null,
      },
    ],
  }),
};

// missing は、ファイルの見つからない録画。すべてないもの、一部だけないもの、保護していて消さないものを並べる。
const missing = {
  missingRecordings: () => [
    {
      recording: {
        id: 'Recording:missing-1',
        name: '架空のドラマ　第１話',
        recordedFrom: at(1),
        protected: false,
      },
      segments: 1,
      missingSegments: 1,
    },
    {
      recording: {
        id: 'Recording:missing-2',
        name: '架空のニュース',
        recordedFrom: at(2),
        protected: false,
      },
      segments: 3,
      missingSegments: 1,
    },
    {
      recording: {
        id: 'Recording:missing-3',
        name: '架空のアニメ　第５話',
        recordedFrom: at(3),
        protected: true,
      },
      segments: 2,
      missingSegments: 2,
    },
  ],
  cleanUpMissingRecordings: () => ({ deletedRecordings: 1, prunedSegments: 1, skipped: 0 }),
};

// orphans は、DB にないファイル。プールのファイルと、データディレクトリの生成物のディレクトリを並べる。
const orphans = {
  orphanFiles: () => [
    {
      poolId: 'default',
      path: '2026/11/架空の番組.m2ts',
      segmentDir: false,
      size: 3.2 * GB,
      modifiedAt: at(5),
    },
    {
      poolId: null,
      path: 'segments/0123456789abcdef',
      segmentDir: true,
      size: 12_000_000,
      modifiedAt: at(6),
    },
  ],
  deleteOrphanFiles: () => ({ deleted: 2, skipped: 0 }),
};

// renames は、今のテンプレートで付け直すと名前が変わる録画。
const renames = {
  recordingRenames: () => [
    {
      recording: { id: 'Recording:rename-1', name: '架空のドラマ　第２話' },
      files: [{ from: '架空のドラマ 第2話.m2ts', to: '2026/11/架空のドラマ 第2話.m2ts' }],
    },
    {
      recording: { id: 'Recording:rename-2', name: '架空の特番' },
      files: [
        { from: '架空の特番.m2ts', to: '2026/11/架空の特番.m2ts' },
        { from: '架空の特番-1.m2ts', to: '2026/11/架空の特番-1.m2ts' },
      ],
    },
  ],
  renameRecordingFiles: () => ({ renamed: 2, skipped: 0, failed: 0 }),
};

// artifacts は、ライブラリの生成物がある状態。
const artifacts = {
  libraryArtifacts: () => ({ size: 250_000_000, files: 1200 }),
  clearLibraryArtifacts: () => ({ size: 250_000_000, files: 1200 }),
};

// clickButton は、名前が name のボタンのうち、index 番目を押す。探すのボタンは、録画とファイルの欄で同じ名前なので並びで選ぶ。
async function clickButton(canvas: Canvas, name: string, index = 0) {
  const button = (await canvas.findAllByRole('button', { name }))[index];
  if (button === undefined) {
    throw new Error('the story could not find button');
  }
  await userEvent.click(button);
}

// confirmDialog は、確かめるダイアログの name のボタンを押す。
async function confirmDialog(name: string) {
  const dialog = await within(document.body).findByRole('dialog');
  await userEvent.click(within(dialog).getByRole('button', { name }));
}

const searchMissing = (canvas: Canvas) => clickButton(canvas, '探す', 0);
const searchOrphans = (canvas: Canvas) => clickButton(canvas, '探す', 1);
const searchRenames = (canvas: Canvas) => clickButton(canvas, '付け直す録画を探す');
const measureArtifacts = (canvas: Canvas) => clickButton(canvas, '片付ける');

export const Default: Story = { args: { overrides: pools } };

export const Loading: Story = { args: { hold: holdAll } };

// 管理者でないとき (サーバーが FORBIDDEN で拒んだ)。
export const Forbidden: Story = {
  args: { overrides: { systemStatus: failingWith({ code: 'FORBIDDEN' }) } },
};

// 保存先を読めなかったとき。
export const Failed: Story = { args: { overrides: { systemStatus: failing() } } };

// このインスタンスに保存先プールがないとき。
export const NoPools: Story = {};

// 空き容量より大きい閾値を打ち、すぐには保存せず確かめるとき。
export const ConfirmMinFree: Story = {
  args: { overrides: pools },
  play: async ({ canvas }) => {
    const [field] = await canvas.findAllByRole('textbox', { name: '容量の閾値（GB）' });
    if (field === undefined) {
      throw new Error('the story could not find field');
    }
    await userEvent.clear(field);
    await userEvent.type(field, '500');
    await userEvent.tab();
  },
};

// 保存先プールの設定を保存できなかったとき (失敗の知らせを出す)。
export const SaveFailed: Story = {
  args: { overrides: { ...pools, updateStoragePool: failing() } },
  play: async ({ canvas }) => {
    const [field] = await canvas.findAllByRole('textbox', { name: '優先順位' });
    if (field === undefined) {
      throw new Error('the story could not find field');
    }
    await userEvent.clear(field);
    await userEvent.type(field, '5');
    await userEvent.tab();
  },
};

// ファイルの見つからない録画を探し、見つからなかったとき。
export const MissingNone: Story = {
  args: { overrides: pools },
  play: async ({ canvas }) => searchMissing(canvas),
};

// ファイルの見つからない録画が見つかったとき。保護した録画は選べない。
export const MissingFound: Story = {
  args: { overrides: { ...pools, ...missing } },
  play: async ({ canvas }) => searchMissing(canvas),
};

// 見つかった録画がどれも保護していて、消すボタンを押せないとき。
export const MissingAllProtected: Story = {
  args: {
    overrides: {
      ...pools,
      missingRecordings: () =>
        missing.missingRecordings().map((item) => ({
          ...item,
          recording: { ...item.recording, protected: true },
        })),
    },
  },
  play: async ({ canvas }) => searchMissing(canvas),
};

// ファイルの見つからない録画を探せなかったとき。
export const MissingSearchFailed: Story = {
  args: { overrides: { ...pools, missingRecordings: failing() } },
  play: async ({ canvas }) => searchMissing(canvas),
};

// ファイルの見つからない録画を消す前に確かめるとき。
export const MissingConfirm: Story = {
  args: { overrides: { ...pools, ...missing } },
  play: async ({ canvas }) => {
    await searchMissing(canvas);
    await clickButton(canvas, '選んだ 2 件をDBから消す');
  },
};

// ファイルの見つからない録画を消し、結果を出したとき。
export const MissingCleaned: Story = {
  args: { overrides: { ...pools, ...missing } },
  play: async (context) => {
    await MissingConfirm.play?.(context);
    await confirmDialog('消す');
  },
};

// ファイルの見つからない録画を消せなかったとき。
export const MissingCleanUpFailed: Story = {
  args: { overrides: { ...pools, ...missing, cleanUpMissingRecordings: failing() } },
  play: MissingCleaned.play,
};

// DB にないファイルを探し、見つからなかったとき。
export const OrphansNone: Story = {
  args: { overrides: pools },
  play: async ({ canvas }) => searchOrphans(canvas),
};

// DB にないファイルが見つかったが、まだ選んでおらず、消すボタンを押せないとき (はじめは何も選ばない)。
export const OrphansNoneSelected: Story = {
  args: { overrides: { ...pools, ...orphans } },
  play: async ({ canvas }) => searchOrphans(canvas),
};

// DB にないファイルが見つかり、一部だけ選んだとき (すべて選ぶの欄は途中の印)。
export const OrphansFound: Story = {
  args: { overrides: { ...pools, ...orphans } },
  play: async ({ canvas }) => {
    await searchOrphans(canvas);
    const [first] = await canvas.findAllByRole('checkbox', { name: /^default:.* を選ぶ$/ });
    if (first === undefined) {
      throw new Error('the story could not find first');
    }
    await userEvent.click(first);
  },
};

// DB にないファイルを探せなかったとき。
export const OrphansSearchFailed: Story = {
  args: { overrides: { ...pools, orphanFiles: failing() } },
  play: async ({ canvas }) => searchOrphans(canvas),
};

// DB にないファイルをすべて選び、消す前に確かめるとき。
export const OrphansConfirm: Story = {
  args: { overrides: { ...pools, ...orphans } },
  play: async ({ canvas }) => {
    await searchOrphans(canvas);
    await userEvent.click(await canvas.findByRole('checkbox', { name: 'すべて選ぶ' }));
    await clickButton(canvas, '選んだ 2 件を消す');
  },
};

// DB にないファイルを消し、結果を出したとき。
export const OrphansDeleted: Story = {
  args: { overrides: { ...pools, ...orphans } },
  play: async (context) => {
    await OrphansConfirm.play?.(context);
    await confirmDialog('消す');
  },
};

// DB にないファイルを消せなかったとき。
export const OrphansDeleteFailed: Story = {
  args: { overrides: { ...pools, ...orphans, deleteOrphanFiles: failing() } },
  play: OrphansDeleted.play,
};

// ライブラリの生成物を数え、消す前に確かめるとき。
export const ArtifactsConfirm: Story = {
  args: { overrides: { ...pools, ...artifacts } },
  play: async ({ canvas }) => measureArtifacts(canvas),
};

// ライブラリの生成物がなく、消すボタンを押せないとき。
export const ArtifactsNothing: Story = {
  args: { overrides: pools },
  play: async ({ canvas }) => measureArtifacts(canvas),
};

// ライブラリの生成物を数えられなかったとき。
export const ArtifactsMeasureFailed: Story = {
  args: { overrides: { ...pools, libraryArtifacts: failing() } },
  play: async ({ canvas }) => measureArtifacts(canvas),
};

// データディレクトリを設定しておらず、片付けられないとき (CONFLICT)。
export const ArtifactsNoDataDir: Story = {
  args: { overrides: { ...pools, libraryArtifacts: failingWith({ code: 'CONFLICT' }) } },
  play: async ({ canvas }) => measureArtifacts(canvas),
};

// ライブラリの生成物を消し、結果を出したとき。
export const ArtifactsCleared: Story = {
  args: { overrides: { ...pools, ...artifacts } },
  play: async ({ canvas }) => {
    await measureArtifacts(canvas);
    await confirmDialog('消す');
  },
};

// 数えた後に、ほかで片付けが済み、消す生成物がなかったとき。
export const ArtifactsClearedNothing: Story = {
  args: {
    overrides: { ...pools, ...artifacts, clearLibraryArtifacts: () => ({ size: 0, files: 0 }) },
  },
  play: ArtifactsCleared.play,
};

// ほかの片付けが終わっておらず、消せなかったとき (CONFLICT)。
export const ArtifactsBusy: Story = {
  args: {
    overrides: { ...pools, ...artifacts, clearLibraryArtifacts: failingWith({ code: 'CONFLICT' }) },
  },
  play: ArtifactsCleared.play,
};

// ライブラリの生成物を消せなかったとき。
export const ArtifactsClearFailed: Story = {
  args: { overrides: { ...pools, ...artifacts, clearLibraryArtifacts: failing() } },
  play: ArtifactsCleared.play,
};

// 付け直すと名前が変わる録画を探し、見つからなかったとき。
export const RenamesNone: Story = {
  args: { overrides: pools },
  play: async ({ canvas }) => searchRenames(canvas),
};

// 付け直すと名前が変わる録画が見つかったとき (今の名前と新しい名前)。
export const RenamesFound: Story = {
  args: { overrides: { ...pools, ...renames } },
  play: async ({ canvas }) => searchRenames(canvas),
};

// 見つかった録画の選択をすべて外し、付け直すボタンを押せないとき。
export const RenamesNoneSelected: Story = {
  args: { overrides: { ...pools, ...renames } },
  play: async ({ canvas }) => {
    await searchRenames(canvas);
    for (const checkbox of await canvas.findAllByRole('checkbox', { name: /を選ぶ$/ })) {
      await userEvent.click(checkbox);
    }
  },
};

// 付け直す録画を探せなかったとき。
export const RenamesSearchFailed: Story = {
  args: { overrides: { ...pools, recordingRenames: failing() } },
  play: async ({ canvas }) => searchRenames(canvas),
};

// 付け直す前に確かめるとき。
export const RenamesConfirm: Story = {
  args: { overrides: { ...pools, ...renames } },
  play: async ({ canvas }) => {
    await searchRenames(canvas);
    await clickButton(canvas, '選んだ 2 件を付け直す');
  },
};

// 付け直している途中 (進み具合を出し、探すボタンと選ぶ欄を押せない)。
export const RenamesRunning: Story = {
  args: {
    overrides: { ...pools, ...renames },
    hold: (operation) => operation.name === 'RecordingRenamesMutation',
  },
  play: async (context) => {
    await RenamesConfirm.play?.(context);
    await confirmDialog('付け直す');
  },
};

// 付け直し、結果を出したとき。
export const RenamesDone: Story = {
  args: { overrides: { ...pools, ...renames } },
  play: RenamesRunning.play,
};

// 付け直したが、失敗した録画があったとき (失敗の色で出す)。
export const RenamesPartlyFailed: Story = {
  args: {
    overrides: {
      ...pools,
      ...renames,
      renameRecordingFiles: () => ({ renamed: 1, skipped: 0, failed: 1 }),
    },
  },
  play: RenamesRunning.play,
};

// 付け直せなかったとき。
export const RenamesFailed: Story = {
  args: { overrides: { ...pools, ...renames, renameRecordingFiles: failing() } },
  play: RenamesRunning.play,
};
