import type { Meta, StoryObj } from '@storybook/react-vite';

import { cancelSave, refreshOfflineRecordings, saveRecording } from '../../offline/store';
import { replaceProperty } from '../conditions';
import { holdAll, PageStory } from '../PageStory';

// オフラインで見る録画 (/offline) の画面 (#2056)。端末に保存した録画の一覧、使っている量、保存の永続化を断られたとき、
// 保存している途中 (大きさが分かるか)、何も保存していないときの表示を並べる。既定は、Storybook を開いたブラウザに
// 保存したものを読む。ほかの Story は、端末の保存先 (navigator.storage) を、メモリの上の偽物に差し替える。
const meta = {
  title: 'ページ/オフライン',
  component: PageStory,
  args: { url: '/offline' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

type SavedRecording = {
  id: string;
  name: string;
  serviceName: string;
  startAt: string;
  size: number;
};

const recordings: SavedRecording[] = [
  {
    id: 'Recording:offline-1',
    name: '[字]ドキュメンタリー「森の一年」',
    serviceName: 'デモ総合',
    startAt: '2026-11-20T21:00:00+09:00',
    size: 3_200_000_000,
  },
  {
    id: 'Recording:offline-2',
    name: '料理の時間',
    serviceName: 'デモ教育',
    startAt: '2026-11-18T18:00:00+09:00',
    size: 850_000_000,
  },
];

// metaText は、保存した録画の情報 (meta.json) の本文。
const metaText = (recording: SavedRecording) =>
  JSON.stringify({
    version: 1,
    ...recording,
    endAt: recording.startAt,
    recordedFrom: recording.startAt,
    recordedUntil: recording.startAt,
    gaps: [],
    jikkyo: false,
    timeZone: 'Asia/Tokyo',
    savedAt: '2026-11-21T00:00:00+09:00',
  });

// fakeDirectory は、files の本文を持つ、録画のディレクトリの偽物。書き込みは、いつまでも終わらない (保存している途中を見せる)。
function fakeDirectory(files: Map<string, string>) {
  return {
    kind: 'directory' as const,
    getFileHandle: (name: string, options?: { create?: boolean }) => {
      const text = files.get(name);
      if (text === undefined && options?.create !== true) {
        return Promise.reject(new DOMException('not found', 'NotFoundError'));
      }

      return Promise.resolve({
        kind: 'file' as const,
        getFile: () => Promise.resolve(new File([text ?? ''], name)),
        createWritable: () =>
          Promise.resolve(new WritableStream<Uint8Array>({ write: () => new Promise(() => {}) })),
      });
    },
  };
}

// fakeStorage は、端末の保存先 (navigator.storage) の偽物。recordings を保存した録画として返す。persisted は、保存の永続化が
// 許されたか。hanging の録画のディレクトリは、release を呼ぶまで開けない (保存を始めた直後の、大きさの分からない状態を見せる)。
function fakeStorage(saved: SavedRecording[], persisted: boolean, hanging?: string) {
  const directories = new Map(
    saved.map((recording) => [
      recording.id,
      fakeDirectory(new Map([['meta.json', metaText(recording)]])),
    ]),
  );
  let release = () => {};
  const root = {
    entries: async function* () {
      yield* directories;
    },
    getDirectoryHandle: (name: string) => {
      if (name === hanging) {
        return new Promise<never>((_, reject) => {
          release = () => reject(new DOMException('aborted', 'AbortError'));
        });
      }
      const directory = directories.get(name) ?? fakeDirectory(new Map());
      directories.set(name, directory);

      return Promise.resolve(directory);
    },
    removeEntry: (name: string) => {
      directories.delete(name);

      return Promise.resolve();
    },
  };

  return {
    storage: {
      getDirectory: () => Promise.resolve({ getDirectoryHandle: () => Promise.resolve(root) }),
      persisted: () => Promise.resolve(persisted),
      persist: () => Promise.resolve(persisted),
      estimate: () => Promise.resolve({ usage: 4_050_000_000, quota: 64_000_000_000 }),
    },
    release: () => release(),
  };
}

// settling は、前の Story の片付けで読み直している一覧。一覧はモジュールに持つので、読み直し終えてから次の保存先に替える。
// 待たないと、前の Story の読み直しが、次の Story の一覧を後から上書きする。
let settling: Promise<unknown> = Promise.resolve();
const reread = () => refreshOfflineRecordings().catch(() => {});

// withFakeStorage は、端末の保存先を偽物にして、一覧を読み直す。片付けで、元の保存先に戻して読み直す。
async function withFakeStorage(saved: SavedRecording[], persisted = true) {
  await settling;
  const { storage } = fakeStorage(saved, persisted);
  const restore = replaceProperty(navigator, 'storage', storage);
  await reread();

  return () => {
    restore();
    settling = reread();
  };
}

// withSaving は、端末の保存先を偽物にして、録画 id を保存し始める。保存は、片付けで止めるまで終わらない。
// started なら、映像を読み始めたところで止める (大きさが分かる)。そうでなければ、読み始める前で止める。
async function withSaving(id: string, started: boolean) {
  await settling;
  const { storage, release } = fakeStorage(recordings, true, started ? undefined : id);
  const restore = replaceProperty(navigator, 'storage', storage);
  // 映像は、同じオリジンの blob: の URL から読ませる (デモの応答の外なので、そのまま届く)。
  const video = URL.createObjectURL(new Blob([new Uint8Array(8_000_000)]));
  await reread();
  const saving = saveRecording({
    id,
    name: '保存している録画',
    serviceName: 'デモ総合',
    startAt: '2026-11-21T21:00:00+09:00',
    endAt: '2026-11-21T22:00:00+09:00',
    recordedFrom: '2026-11-21T21:00:00+09:00',
    recordedUntil: '2026-11-21T22:00:00+09:00',
    gaps: [],
    streamUrl: video,
    jikkyoCommentsUrl: null,
    files: [],
  });

  // 保存を止め、止まっている処理を進めてから、元の保存先に戻す。保存を終えてから、元の保存先で読み直す。
  return () => {
    cancelSave(id);
    release();
    URL.revokeObjectURL(video);
    restore();
    settling = saving.catch(() => {}).then(reread);
  };
}

// 既定。Storybook を開いたブラウザに保存した録画を読む。
export const Default: Story = {
  beforeEach: async () => {
    await settling;
    await reread();
  },
};

// GraphQL を読まない画面なので、デモの応答を待たせても変わらない。
export const Loading: Story = { args: { hold: holdAll } };

// 保存した録画があるとき。新しく放送したものから並べ、使っている量と上限を出す。
export const Saved: Story = { beforeEach: () => withFakeStorage(recordings) };

// 何も保存していないとき。
export const Empty: Story = { beforeEach: () => withFakeStorage([]) };

// ブラウザが保存の永続化を断ったとき (容量が足りなくなると消されうる)。
export const NotPersisted: Story = { beforeEach: () => withFakeStorage(recordings, false) };

// 保存している途中で、大きさが分かるとき (読み込んだ量と、全体の割合)。
export const SavingWithTotal: Story = {
  beforeEach: () => withSaving('Recording:saving-1', true),
};

// 保存を始めたばかりで、大きさがまだ分からないとき (読み込んだ量だけを出す)。
export const SavingWithoutTotal: Story = {
  beforeEach: () => withSaving('Recording:saving-2', false),
};
