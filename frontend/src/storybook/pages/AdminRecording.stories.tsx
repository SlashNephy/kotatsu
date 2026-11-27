import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent, within } from 'storybook/test';

import type { DemoRoot } from '../../demo';
import { call } from '../conditions';
import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// 録画の設定 (/admin/recording) の画面 (#2056)。録画とルールの既定値と、字幕の自動生成のモデルの保存先の状態を並べる。
const meta = {
  title: 'ページ/管理/録画',
  component: PageStory,
  args: { url: '/admin/recording' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type Settings = Record<string, unknown> & {
  recordingDefaults: Record<string, unknown>;
  ruleDefaults: Record<string, unknown>;
};

type Slot = Record<string, unknown> & {
  kind: string;
  models: { id: string; name: string; size: number }[];
};

// pools は、保存先プールを 2 つ持つ状態。プールのメニューに、自動で選ぶのほかに 2 つ並ぶ。
const pools = { systemStatus: () => ({ pools: [{ id: 'default' }, { id: 'archive' }] }) };

// withSettings は、全体の設定のうち patch の返す項目を変える。
function withSettings(patch: (settings: Settings) => object) {
  return (root: DemoRoot) => ({
    ...pools,
    systemSettings: () => {
      const settings = call<Settings>(root, 'systemSettings');

      return { ...settings, ...patch(settings) };
    },
  });
}

// withSlots は、字幕の自動生成のモデルの保存先を、種類 (WHISPER、VAD) ごとに patches で変える。
function withSlots(patches: Record<string, (slot: Slot) => object>) {
  return (root: DemoRoot) => ({
    ...pools,
    captionModels: () =>
      call<Slot[]>(root, 'captionModels').map((slot) => ({
        ...slot,
        ...patches[slot.kind]?.(slot),
      })),
  });
}

const at = (day: number) => `2026-11-${String(day).padStart(2, '0')}T12:00:00+09:00`;

export const Default: Story = { args: { overrides: pools } };

export const Loading: Story = { args: { hold: holdAll } };

// failingLeaf は、この画面だけが読む項目を fail で失敗させる。ヘッダーも読む全体の設定は、失敗させない。
const failingLeaf = (fail: () => never) => withSettings(() => ({ historyMaxDrops: fail }));

// 管理者でないとき (サーバーが FORBIDDEN で拒んだ)。
export const Forbidden: Story = {
  args: { overrides: failingLeaf(failingWith({ code: 'FORBIDDEN' })) },
};

// 設定を読めなかったとき。
export const Failed: Story = { args: { overrides: failingLeaf(failing()) } };

// 保存先プールがなく、既定のプールも決めていない (自動で選ぶ) とき。プールのメニューは自動で選ぶだけ。
export const NoPools: Story = {
  args: {
    overrides: (root: DemoRoot) => ({
      systemStatus: () => ({ pools: [] }),
      systemSettings: () => {
        const settings = call<Settings>(root, 'systemSettings');

        return { ...settings, recordingDefaults: { ...settings.recordingDefaults, poolId: null } };
      },
    }),
  },
};

// 既定値を変えたことがあり、最終更新の時刻を出すとき。重複を避けるのを既定にしたので、期間の欄も出す。
export const Customized: Story = {
  args: {
    overrides: withSettings((settings) => ({
      updatedAt: at(20),
      cmDetector: 'LOGO',
      tvtplayChaptersEnabled: true,
      thumbnailPreviewsEnabled: true,
      sleepControlEnabled: true,
      captionThreads: 4,
      recordingDefaults: {
        ...settings.recordingDefaults,
        poolId: 'archive',
        startMarginSeconds: 30,
        endMarginSeconds: 60,
        priority: 3,
        detectCm: true,
        generateCaptions: true,
        transcodePreset: '720p',
      },
      ruleDefaults: { ...settings.ruleDefaults, avoidDuplicates: true, duplicateWindowDays: 14 },
    })),
  },
};

// 重複を避けるのを既定にしたが、期間を区切らないとき (期間の欄が空)。
export const AvoidDuplicatesUnbounded: Story = {
  args: {
    overrides: withSettings((settings) => ({
      ruleDefaults: { ...settings.ruleDefaults, avoidDuplicates: true, duplicateWindowDays: null },
    })),
  },
};

// ファイル名のテンプレートを空にして欄を離れ、入力の誤りを出したとき。
export const TemplateRequired: Story = {
  args: { overrides: pools },
  play: async ({ canvas }) => {
    const field = await canvas.findByRole('textbox', { name: 'ファイル名のテンプレート' });
    await userEvent.clear(field);
    await userEvent.tab();
  },
};

// 設定を保存できなかったとき (失敗の通知を出す)。
export const SaveFailed: Story = {
  args: { overrides: { ...pools, updateSystemSettings: failing() } },
  play: async ({ canvas }) => {
    const [first] = await canvas.findAllByRole('switch', {
      name: /^分かれた録画のファイルを1つにつなぐ/,
    });
    if (first === undefined) {
      throw new Error('the story could not find first');
    }
    await userEvent.click(first);
  },
};

// モデルのダウンロードを待っているとき (進み具合はまだない)。
export const CaptionModelQueued: Story = {
  args: {
    overrides: withSlots({
      WHISPER: (slot) => ({
        state: 'DOWNLOADING',
        download: { model: slot.models[0], running: false, failed: false, progress: null },
      }),
    }),
  },
};

// モデルをダウンロードしているとき (進み具合と残り時間)。
export const CaptionModelDownloading: Story = {
  args: {
    overrides: withSlots({
      WHISPER: (slot) => ({
        state: 'DOWNLOADING',
        download: {
          model: slot.models[0],
          running: true,
          failed: false,
          progress: { ratio: 0.42, remainingSeconds: 95, updatedAt: at(22) },
        },
      }),
    }),
  },
};

// 画面から置いたモデルと、手で置いたファイルがあるとき (置き換えると消すのボタンを出す)。
export const CaptionModelPlaced: Story = {
  args: {
    overrides: withSlots({
      WHISPER: (slot) => ({
        state: 'PLACED',
        installed: { model: slot.models[0], size: slot.models[0]?.size ?? 0, installedAt: at(18) },
      }),
      VAD: () => ({
        state: 'PLACED',
        installed: { model: null, size: 885098, installedAt: null },
      }),
    }),
  },
};

// 設定で別のファイルを使っていて、ここに置いたモデルを使わないとき。
export const CaptionModelOverridden: Story = {
  args: {
    overrides: withSlots({
      WHISPER: () => ({ overridden: true }),
      VAD: () => ({ overridden: true }),
    }),
  },
};

// 最後のダウンロードに失敗したとき。前に置いたモデルは残っている。
export const CaptionModelFailed: Story = {
  args: {
    overrides: withSlots({
      WHISPER: (slot) => ({
        state: 'FAILED',
        installed: { model: slot.models[1], size: slot.models[1]?.size ?? 0, installedAt: at(10) },
        download: { model: slot.models[0], running: false, failed: true, progress: null },
      }),
    }),
  },
};

// 選べるモデルがなく、ダウンロードのボタンを押せないとき。
export const CaptionModelNoChoices: Story = {
  args: { overrides: withSlots({ VAD: () => ({ models: [] }) }) },
};

// ダウンロードを頼めなかったとき (欄の中に理由を出す)。
export const CaptionModelDownloadFailed: Story = {
  args: { overrides: { ...pools, downloadCaptionModel: failingWith({ code: 'CONFLICT' }) } },
  play: async ({ canvas }) => {
    const [download] = await canvas.findAllByRole('button', { name: 'ダウンロードする' });
    if (download === undefined) {
      throw new Error('the story could not find download');
    }
    await userEvent.click(download);
  },
};

// 置いたモデルを消す前に確認するとき。
export const CaptionModelDeleting: Story = {
  args: { overrides: CaptionModelPlaced.args?.overrides },
  play: async ({ canvas }) => {
    const [remove] = await canvas.findAllByRole('button', { name: '消す' });
    if (remove === undefined) {
      throw new Error('the story could not find remove');
    }
    await userEvent.click(remove);
    await within(document.body).findByRole('dialog');
  },
};
