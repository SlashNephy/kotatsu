import { execFileSync } from 'node:child_process';
import { readdirSync, readFileSync, realpathSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

import babel from '@rolldown/plugin-babel';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import react, { reactCompilerPreset } from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';
import { defineConfig, lazyPlugins, type Plugin } from 'vite-plus';

// ビルドした SPA は、Go のバイナリに埋め込む (設計書 2.3)。backend/internal/web/webui/embed.go が、その dist/app を埋め込む。
// dist 自体は、未ビルドでも go build が通るように、空の .gitkeep だけを置いてリポジトリに残す。
// devAPI は、開発サーバーが API を中継する先。
// 開発用の Compose では、KOTATSU_DEV_API で Kotatsu のコンテナを指す。
// Compose の外で mise run //frontend:dev を動かすときは、手元で起動した Kotatsu (既定は 127.0.0.1:8080) に中継する。
// 空の値は、渡していないものとして扱う。
const devAPI =
  process.env.KOTATSU_DEV_API || `http://127.0.0.1:${process.env.KOTATSU_DEV_PORT || '8080'}`;

// inlineAppleTouchIcon は、index.html の apple-touch-icon を、public/icons の PNG の data: URL に置き換える (#582)。
// iOS 27 の「ホーム画面に追加」は、URL で指したアイコンを読み込めず、タイトルの頭文字のタイルを出す。
// data: URL なら、別のリクエストなしにアイコンを渡せる。
function inlineAppleTouchIcon(): Plugin {
  const placeholder = '__KOTATSU_APPLE_TOUCH_ICON__';

  return {
    name: 'kotatsu:inline-apple-touch-icon',
    transformIndexHtml(html) {
      const icon = readFileSync(
        new URL('./public/icons/apple-touch-icon-180.png', import.meta.url),
      );

      return html.replace(placeholder, `data:image/png;base64,${icon.toString('base64')}`);
    },
  };
}

// BundledPackage は、画面に同梱するパッケージ 1 つと、そのライセンスの文書。licenses.json に書き、/about の画面が読む (#1604)。
type BundledPackage = {
  name: string;
  version: string;
  license: string;
  homepage: string | null;
  files: { name: string; text: string }[];
  // generated は、パッケージにライセンスの文書がないので、package.json の license と author から作ったときに真になる。
  generated: boolean;
};

// licenseFile は、パッケージの直下にある、ライセンスの文書と NOTICE のファイルの名前。
const licenseFile = /^(licen[cs]e|copying|notice)([.\-_].*)?$/i;

// packagesWithoutLicenseFile は、ライセンスの文書を持たずに公開されているパッケージ。どれも package.json で MIT を宣言しているので、
// MIT の文に author を著作権者として入れて示す。ここにないパッケージで文書が見つからなければ、ビルドを止める。
const packagesWithoutLicenseFile = new Set(['react-remove-scroll-bar', 'tr46']);

const mitLicenseText = (holder: string) => `MIT License

Copyright (c) ${holder}

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`;

// readPackage は、dir のパッケージの package.json とライセンスの文書を読む。
function readPackage(dir: string): BundledPackage {
  const manifest = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8')) as {
    name: string;
    version: string;
    license?: string;
    homepage?: string;
    author?: string | { name?: string };
  };
  const files = readdirSync(dir, { withFileTypes: true })
    .filter((entry) => entry.isFile() && licenseFile.test(entry.name))
    .map((entry) => ({ name: entry.name, text: readFileSync(join(dir, entry.name), 'utf8') }))
    .sort((a, b) => a.name.localeCompare(b.name));
  const license = manifest.license ?? 'UNKNOWN';
  let generated = false;
  if (files.length === 0) {
    const author = typeof manifest.author === 'string' ? manifest.author : manifest.author?.name;
    if (
      !packagesWithoutLicenseFile.has(manifest.name) ||
      license !== 'MIT' ||
      author === undefined
    ) {
      throw new Error(`no license file found for ${manifest.name}@${manifest.version}`);
    }
    files.push({ name: 'LICENSE', text: mitLicenseText(author.replace(/\s*<[^>]*>/, '')) });
    generated = true;
  }

  return {
    name: manifest.name,
    version: manifest.version,
    license,
    homepage: manifest.homepage ?? null,
    files,
    generated,
  };
}

// bundledLicenses は、画面に同梱するパッケージと、そのライセンスの文書の一覧を licenses.json に書き出す (設計書 付録 B、#1604)。
// 同梱するのは、dependencies とその依存 (pnpm licenses list --prod) と、dependencies に置いたワークスペースのパッケージ
// (vendor/ の mpeg2toh264) である。Service Worker に入れる workbox も dependencies にあるので、ここに含まれる。
// 一覧は、依存を上げるたびに変わるので、リポジトリには置かず、ビルドのたびに作る。/about の画面を開いたときだけ読む。
function bundledLicenses(): Plugin {
  const root = fileURLToPath(new URL('.', import.meta.url));

  return {
    name: 'kotatsu:bundled-licenses',
    apply: 'build',
    generateBundle() {
      const listed = JSON.parse(
        execFileSync('pnpm', ['licenses', 'list', '--prod', '--json'], {
          cwd: root,
          encoding: 'utf8',
        }),
      ) as Record<string, { paths: string[] }[]>;
      // paths には、版ごとのディレクトリと、依存の組み合わせごとに同じ版を別に置いたディレクトリが並ぶ。
      // 同じパッケージの同じ版は 1 つにし、版が違えば、それぞれ同梱するので分けて並べる。
      const dirs = Object.values(listed).flatMap((packages) =>
        packages.flatMap((pkg) => pkg.paths),
      );
      const manifest = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8')) as {
        dependencies: Record<string, string>;
      };
      for (const [name, spec] of Object.entries(manifest.dependencies)) {
        if (spec.startsWith('workspace:')) {
          dirs.push(realpathSync(join(root, 'node_modules', name)));
        }
      }
      const unique = new Map(
        dirs.map(readPackage).map((pkg) => [`${pkg.name}@${pkg.version}`, pkg] as const),
      );
      const packages = [...unique.values()].sort(
        (a, b) => a.name.localeCompare(b.name) || a.version.localeCompare(b.version),
      );
      this.emitFile({ type: 'asset', fileName: 'licenses.json', source: JSON.stringify(packages) });
    },
  };
}

// mode が demo のときは、静的なデモ (#832) をビルドする。`vp build --mode demo` で使う。
// demoStream は、静的なデモ (#832) のときだけ、合成の映像 (demo/stream.m2ts) と、
// ライブ視聴と録画の再生で映像を繰り返して流す Service Worker (stream-sw.js と、それが読む demo/stream-loop.js) を出力に加える。
// public に置くと、tenban に埋め込む通常のビルドにも入るので、ここで加える。
// Service Worker は、デモの全体を受け持てるよう、base の直下に置く。
function demoStream(): Plugin {
  const files = [
    { fileName: 'demo/stream.m2ts', source: './demo/stream.m2ts', type: 'video/mp2t' },
    { fileName: 'stream-sw.js', source: './demo/stream-sw.js', type: 'text/javascript' },
    { fileName: 'demo/stream-loop.js', source: './demo/stream-loop.js', type: 'text/javascript' },
  ];

  return {
    name: 'kotatsu:demo-stream',
    // 開発サーバー (vp dev --mode demo) でも、同じパスで配信する。
    configureServer(server) {
      const base = server.config.base;
      server.middlewares.use((request, response, next) => {
        const path = (request.url ?? '').split('?')[0];
        const file = files.find((item) => path === `${base}${item.fileName}`);
        if (file === undefined) {
          next();

          return;
        }
        response.setHeader('Content-Type', file.type);
        response.end(readFileSync(new URL(file.source, import.meta.url)));
      });
    },
    generateBundle() {
      for (const file of files) {
        this.emitFile({
          type: 'asset',
          fileName: file.fileName,
          source: readFileSync(new URL(file.source, import.meta.url)),
        });
      }
    },
  };
}

export default defineConfig(({ mode }) => ({
  plugins: lazyPlugins(() => [
    // Relay のコンパイラが routes/ に生成する __generated__ は、ルートではない。
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
      routeFileIgnorePattern: '__generated__',
    }),
    react(),
    inlineAppleTouchIcon(),
    bundledLicenses(),
    ...(mode === 'demo' ? [demoStream()] : []),
    // graphql`...` を、Relay のコンパイラが生成したアーティファクトの import に置き換える。
    babel({ presets: [reactCompilerPreset()], plugins: [['relay', { eagerEsModules: true }]] }),
    // PWA の外枠 (設計書 10.1)。Web Push の受信とナビゲーションは、自前の Service Worker (src/sw.ts) に書く。
    // デモは Service Worker を登録しないので、作らない。
    VitePWA({
      disable: mode === 'demo',
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      // 登録は main.tsx で virtual:pwa-register から行う。CSP の nonce のない inline script を差し込ませない。
      injectRegister: false,
      injectManifest: {
        // 外枠だけを対象にする。見出しのフォントは、unicode-range ごとに 100 以上のファイルに分かれて
        // 3 MB ほどになるので含めない。オフラインでは、見出しもシステムフォントで出る。
        // ブラウザの中での変換の WebAssembly (500 KB ほど) は、端末に保存した録画 (#480) をオフラインで再生するのに要るので含める。
        globPatterns: ['**/*.{js,css,html,png,svg,webmanifest,wasm}'],
        // パスワードの強度の判定の辞書 (2 MB ほど) は、セットアップとパスワードの変更でしか使わないので含めない。
        globIgnores: ['**/zxcvbn-*.js'],
      },
      manifest: {
        name: 'Kotatsu',
        short_name: 'Kotatsu',
        description: 'テレビの予約、録画、視聴をまとめるアプリ',
        lang: 'ja',
        // マニフェストの URL から解決させ、Vite の base に置いても同じ場所を指すようにする (#832)。
        start_url: '.',
        scope: '.',
        display: 'standalone',
        theme_color: '#ff7407',
        background_color: '#fbf7ef',
        icons: [
          { src: 'icons/icon-192.png', sizes: '192x192', type: 'image/png' },
          { src: 'icons/icon-512.png', sizes: '512x512', type: 'image/png' },
          {
            src: 'icons/icon-maskable-512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
    }),
  ]),
  html: {
    // tenban が index.html を配信するたびに、リクエストごとの nonce に置き換える (設計書 10.4)。
    cspNonce: '__KOTATSU_CSP_NONCE__',
  },
  build: {
    outDir: '../backend/internal/web/webui/dist/app',
    emptyOutDir: true,
    // CSP の font-src は 'self' だけなので、小さいフォントも data: URL にせず、ファイルのまま配信する。
    assetsInlineLimit: (file) => (/\.(woff2?|ttf|otf)$/.test(file) ? false : undefined),
  },
  server: {
    // 開発中は、API を手元の tenban に中継する。compose.yaml と同じく、KOTATSU_DEV_PORT でポートを変えられる。
    // Host は書き換えない。tenban は、Host から求めたオリジンと Origin を比べて /internal/auth/* の CSRF を防ぎ、
    // OIDC のリダイレクト URI も Host から作る。文字列で書くと changeOrigin が有効になり、どちらも食い違う。
    proxy: {
      // 画面がセッションの Cookie で使う端点 (GraphQL、ログイン、映像、画像、#1605) を中継する。
      // 実況のコメントの中継 (/internal/media/jikkyo/*/ws) は WebSocket なので、ws も中継する。
      // 映像、録画の画像、ロゴも中継し、開発サーバーだけで画面のすべてを確かめられるようにする。
      '/internal/': { target: devAPI, changeOrigin: false, ws: true },
      // ブラウザ以外から叩く端点 (EPGStation 互換の API、予約の iCalendar、IPTV、署名付き URL の配信、#1605) も、
      // 開発サーバーから確かめられるようにする。
      '/api': { target: devAPI, changeOrigin: false },
      // MCP のクライアントにトークンを発行する認可サーバー (#963) も、開発サーバーから確かめられるようにする。
      '/oauth': { target: devAPI, changeOrigin: false },
      '/.well-known': { target: devAPI, changeOrigin: false },
      // MCP サーバー (#964) も、開発サーバーから確かめられるようにする。
      '/mcp': { target: devAPI, changeOrigin: false },
    },
  },
  fmt: {
    singleQuote: true,
    // vendor/ は、上流のソースからビルドした成果物なので、整形しない (scripts/build-mpeg2toh264.sh)。
    ignorePatterns: [
      'src/routeTree.gen.ts',
      // internal/domain/printenclosed が書き出す (mise run //backend:generate)。
      'src/futon/enclosedCharacters.gen.ts',
      'schema.graphql',
      '**/__generated__/**',
      'vendor/**',
    ],
  },
  lint: {
    ignorePatterns: [
      'src/routeTree.gen.ts',
      'dist/**',
      'storybook-static/**',
      '**/__generated__/**',
      'vendor/**',
    ],
    options: {
      typeAware: true,
      typeCheck: true,
    },
    rules: {
      // useLazyLoadQuery は、ErrorBoundary が戻したときに読み直す relay/useLazyLoadQuery を使う。
      'no-restricted-imports': [
        'error',
        {
          paths: [
            {
              name: 'react-relay',
              importNames: ['useLazyLoadQuery'],
              message: 'Use useLazyLoadQuery from src/relay/useLazyLoadQuery instead.',
            },
          ],
        },
      ],
    },
    overrides: [
      {
        files: ['src/relay/useLazyLoadQuery.ts'],
        rules: { 'no-restricted-imports': 'off' },
      },
    ],
  },
  test: {
    include: ['src/**/*.test.{ts,tsx}'],
    environment: 'jsdom',
    setupFiles: ['src/test/setup.ts'],
  },
}));
