import type { Meta, StoryObj } from '@storybook/react-vite';

import { failing, failingWith, holdAll, PageStory } from '../PageStory';

// ライブラリ (/library) の画面 (#2056)。ライブラリの一覧、フォルダの中身、名前で探した結果と、それぞれが空のとき、
// 読み込めなかったときの表示を並べる。デモにはライブラリがないので、既定は空の一覧になる。
const meta = {
  title: 'ページ/ライブラリ',
  component: PageStory,
  args: { url: '/library' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

const modifiedAt = '2026-11-20T21:00:00+09:00';

const libraries = () => [
  { name: 'pool', kind: 'POOL' },
  { name: 'archive', kind: 'FOLDER' },
  { name: 'drive', kind: 'GOOGLE_DRIVE' },
];

const file = (path: string, size: number) => ({
  name: path.split('/').at(-1) ?? path,
  path,
  directory: false,
  size,
  modifiedAt,
});

const folder = (path: string) => ({
  name: path.split('/').at(-1) ?? path,
  path,
  directory: true,
  size: 0,
  modifiedAt,
});

// directory は、ライブラリ archive のフォルダ path の中身を返す。
const directory = ({ path }: { path: string }) =>
  path === ''
    ? {
        path: '',
        names: [],
        entries: [
          folder('バラエティ'),
          folder('特番'),
          file('ニュース 2026-11-01.m2ts', 1_800_000_000),
          file('天気予報 2026-11-02.m2ts', 300_000_000),
        ],
      }
    : {
        path,
        names: path.split('/'),
        entries: [folder(`${path}/秋`), file(`${path}/ドキュメンタリー 第1回.m2ts`, 6_400_000_000)],
      };

export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// ライブラリがあるとき。保存先のプール、フォルダ、Google Drive の種類を出す。
export const Libraries: Story = { args: { overrides: { libraries } } };

// ライブラリの根元を開いたとき (上のフォルダへのリンクを出さない)。
export const Root: Story = {
  args: { url: '/library?library=archive', overrides: { libraryDirectory: directory } },
};

// ライブラリの中のフォルダを開いたとき (パンくずと、上のフォルダへのリンク)。
export const Folder: Story = {
  args: {
    url: '/library?library=archive&path=バラエティ',
    overrides: { libraryDirectory: directory },
  },
};

// 空のフォルダを開いたとき。
export const EmptyFolder: Story = {
  args: {
    url: '/library?library=archive&path=empty',
    overrides: { libraryDirectory: () => ({ path: 'empty', names: ['empty'], entries: [] }) },
  },
};

// 名前で探したとき。同じ名前のファイルを見分けられるよう、あるフォルダを添える。探す語を消すボタンも出す。
export const Search: Story = {
  args: {
    url: '/library?library=archive&q=ニュース',
    overrides: {
      libraryDirectory: directory,
      librarySearch: () => ({
        truncated: false,
        hits: [
          { folderNames: [], entry: file('ニュース 2026-11-01.m2ts', 1_800_000_000) },
          {
            folderNames: ['バラエティ'],
            entry: file('バラエティ/ニュース 2026-11-01.m2ts', 1_700_000_000),
          },
        ],
      }),
    },
  },
};

// 探した結果が多すぎて、途中で打ち切ったとき。
export const SearchTruncated: Story = {
  args: {
    url: '/library?library=archive&q=m2ts',
    overrides: {
      libraryDirectory: directory,
      librarySearch: () => ({
        truncated: true,
        hits: [{ folderNames: [], entry: file('ニュース 2026-11-01.m2ts', 1_800_000_000) }],
      }),
    },
  },
};

// 探しても見つからなかったとき。
export const SearchEmpty: Story = {
  args: {
    url: '/library?library=archive&q=該当なし',
    overrides: {
      libraryDirectory: directory,
      librarySearch: () => ({ truncated: false, hits: [] }),
    },
  },
};

// ないライブラリかフォルダを開いたとき。
export const NotFound: Story = {
  args: {
    url: '/library?library=missing',
    overrides: { libraryDirectory: failingWith({ code: 'NOT_FOUND' }) },
  },
};

// Google Drive が混んでいて、読めなかったとき。
export const Busy: Story = {
  args: {
    url: '/library?library=drive',
    overrides: { libraryDirectory: failingWith({ code: 'CONFLICT' }) },
  },
};

// そのほかの理由で読めなかったとき。
export const Error: Story = { args: { overrides: { libraries: failing() } } };
