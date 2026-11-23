import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { paginate } from '../../demo/data';
import { GraphQLRequestError } from '../../relay/requestError';
import { failing, holdAll, PageStory } from '../PageStory';

// キャプチャの一覧 (/captures) の画面 (#2056)。録画で撮ったもの、ライブで撮ったもの、番組名のないもの、続きの読み込み、
// 探して当たらないとき、サーバーに残せない設定のとき、操作に失敗したときの表示を並べる。
const meta = {
  title: 'ページ/キャプチャ',
  component: PageStory,
  args: { url: '/captures' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// image は、キャプチャの画像の代わりに出す、16:9 の色の付いた SVG。
const image = (color: string) =>
  `data:image/svg+xml;charset=utf-8,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><rect width="320" height="180" fill="${color}"/></svg>`,
  )}`;

const captures = [
  // 録画の 12 分 34 秒で撮ったもの (撮った場面から再生できる)。
  {
    id: 'Capture:1',
    imageUrl: image('#e8743b'),
    serviceName: 'デモ総合',
    programName: '[字]ドキュメンタリー「海の生き物」',
    recordingId: 'Recording:1',
    position: 754,
    broadcastAt: '2026-11-20T21:12:34+09:00',
  },
  // 録画で撮ったが、撮った位置が分からないもの (録画の詳細を開く)。
  {
    id: 'Capture:2',
    imageUrl: image('#19a979'),
    serviceName: 'デモ教育',
    programName: '料理の時間',
    recordingId: 'Recording:2',
    position: null,
    broadcastAt: '2026-11-19T18:05:00+09:00',
  },
  // ライブで撮ったもので、番組名が分からないもの (サービス名だけ)。
  {
    id: 'Capture:3',
    imageUrl: image('#3b6fe8'),
    serviceName: 'デモBS',
    programName: null,
    recordingId: null,
    position: null,
    broadcastAt: '2026-11-18T04:30:00+09:00',
  },
];

type Args = { first: number; after?: string | null; keyword?: string | null };

// listed は、キャプチャの一覧を、first 件ずつのページで返す。
const listed = (items: typeof captures) => (args: Args) => paginate(items, args.first, args.after);

// signedOut は、キャプチャの一覧の読み込みを、ログインしていないときの拒否 (HTTP 401) で失敗させる。
const signedOut = (operation: { name: string }) => {
  if (operation.name === 'capturesListQuery') {
    throw new GraphQLRequestError('unauthenticated', 401, 'UNAUTHENTICATED');
  }

  return false;
};

// many は、続きのあるだけのキャプチャ (1 ページは 30 件)。
const many = Array.from({ length: 40 }, (_, index) =>
  captures.map((capture) => ({ ...capture, id: `${capture.id}-${index}` })),
).flat();

// 既定。デモの撮ったキャプチャはまだないので、空の一覧になる。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

export const Error: Story = { args: { overrides: { captures: failing() } } };

// ログインしていないとき (ログインへのリンク)。
export const SignedOut: Story = { args: { hold: signedOut } };

// キャプチャがあるとき。録画の撮った場面から再生するリンク、録画を開くリンク、番組名のないもの。
export const Captures: Story = { args: { overrides: { captures: listed(captures) } } };

// 探す語に当たるキャプチャがないとき。
export const NoMatch: Story = {
  args: { overrides: { captures: listed([]) } },
  play: async ({ canvas }) => {
    await userEvent.type(
      await canvas.findByRole('textbox', { name: '番組名、サービス名、字幕で探す' }),
      '該当なし',
    );
  },
};

// 管理者がキャプチャをサーバーに残さない設定にしているとき。
export const Unavailable: Story = {
  args: { overrides: { capturesAvailable: () => false, captures: listed([]) } },
};

// 続きがあるとき (さらに読み込むボタン)。
export const More: Story = { args: { overrides: { captures: listed(many) } } };

// 続きを読み込めなかったとき。
export const MoreFailed: Story = {
  args: {
    overrides: {
      captures: (args: Args) => {
        if (args.after != null) {
          throw new globalThis.Error('the demo server failed');
        }

        return paginate(many, args.first, args.after);
      },
    },
  },
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: 'さらに読み込む' }));
  },
};

// 消せなかったとき (カードの下に理由を出す)。
export const DeleteFailed: Story = {
  args: { overrides: { captures: listed(captures), deleteCapture: failing() } },
  play: async ({ canvas }) => {
    const [first] = await canvas.findAllByRole('button', { name: '削除' });
    if (first !== undefined) {
      await userEvent.click(first);
    }
  },
};
