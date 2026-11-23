import type { Meta, StoryObj } from '@storybook/react-vite';
import { userEvent } from 'storybook/test';

import { failing, holdAll, PageStory } from '../PageStory';

// Kotatsu について (/about) の画面 (#2056)。バージョン、同梱するソフトウェアのライセンスの一覧 (出力していないビルド、読んでいる途中、
// 絞り込んで当たらないとき) と、読み込めなかったときの表示を並べる。
const meta = {
  title: 'ページ/Kotatsuについて',
  component: PageStory,
  args: { url: '/about' },
  parameters: { layout: 'fullscreen' },
} satisfies Meta<typeof PageStory>;

export default meta;

type Story = StoryObj<typeof meta>;

// serving は、serveFiles が応える、パスの末尾 (licenses.json など) ごとの本文。null なら、応えずに待たせる。
// 静的にビルドした Storybook では BASE_URL が相対になるので、パスの末尾で見分ける。
let serving: Record<string, string | null> = {};

// serveFiles は、ビルドで出力するライセンスの一覧などのファイルを、files の本文で返すように fetch を包む。
// デモの応答 (interceptFetch) は、Story を描くときに、ここで包んだ fetch をさらに包む。包まれたまま残っても、片付けた後は
// 何も返さずに元の fetch へ渡す。
function serveFiles(files: Record<string, string | null>) {
  return () => {
    serving = files;
    const previous = window.fetch;
    const wrapped: typeof window.fetch = (input, init) => {
      const url = new URL(input instanceof Request ? input.url : input, window.location.href);
      const path = Object.keys(serving).find((key) => url.pathname.endsWith(`/${key}`));
      const body = path === undefined ? undefined : serving[path];
      if (url.origin !== window.location.origin || body === undefined) {
        return previous(input, init);
      }
      if (body === null) {
        return new Promise<Response>(() => {});
      }

      return Promise.resolve(
        new Response(body, {
          headers: {
            'Content-Type': (path ?? '').endsWith('.json') ? 'application/json' : 'text/plain',
          },
        }),
      );
    };
    window.fetch = wrapped;

    return () => {
      serving = {};
      if (window.fetch === wrapped) {
        window.fetch = previous;
      }
    };
  };
}

const bundled = JSON.stringify([
  {
    name: 'example-ui',
    version: '1.2.3',
    license: 'MIT',
    homepage: 'https://example.com/example-ui',
    files: [{ name: 'LICENSE', text: 'MIT License\n\nCopyright (c) Example Authors' }],
    generated: false,
  },
  {
    name: 'example-codec',
    version: '0.4.0',
    license: 'BSD-3-Clause',
    homepage: null,
    files: [],
    generated: true,
  },
]);

const goModuleLicenses = () => [
  { path: 'std', version: 'go1.26.0', files: [{ name: 'LICENSE', text: 'BSD-style license' }] },
  {
    path: 'example.com/server',
    version: 'v1.0.0',
    files: [{ name: 'LICENSE', text: 'Apache License 2.0' }],
  },
];

// 既定。ビルドがライセンスの一覧を出力していないので、フロントエンドとバックエンドの一覧は「書き出していない」と出す。
export const Default: Story = {};

export const Loading: Story = { args: { hold: holdAll } };

// 読み込めなかったとき。
export const Error: Story = { args: { overrides: { version: failing() } } };

// リリースしたビルド。バージョンとコミットが分かり、ライセンスの一覧も出力してある。
export const Released: Story = {
  args: {
    overrides: {
      version: () => ({
        version: 'v2026.11.0',
        revision: '0123456789abcdef',
        modified: false,
        latestRelease: null,
      }),
      goModuleLicenses,
    },
  },
  beforeEach: serveFiles({
    'licenses.json': bundled,
    'licenses/libaribcaption.txt': 'libaribcaption の注記の例',
  }),
};

// 一覧の行を開いたとき (ライセンスの文書、ホームページ、ビルドで作った文書であることの注記)。
export const Expanded: Story = {
  args: { overrides: { goModuleLicenses } },
  beforeEach: serveFiles({ 'licenses.json': bundled }),
  play: async ({ canvas }) => {
    await userEvent.click(await canvas.findByRole('button', { name: /example-ui/ }));
    await userEvent.click(await canvas.findByRole('button', { name: /example-codec/ }));
  },
};

// フロントエンドのライセンスの一覧を読んでいる途中。
export const LicensesLoading: Story = { beforeEach: serveFiles({ 'licenses.json': null }) };

// 絞り込んで、どの行も当たらないとき。
export const NoMatch: Story = {
  args: { overrides: { goModuleLicenses } },
  beforeEach: serveFiles({ 'licenses.json': bundled }),
  play: async ({ canvas }) => {
    await userEvent.type(await canvas.findByRole('textbox', { name: '絞り込み' }), '該当なし');
  },
};
