// 静的なデモ (#832) と、Storybook の画面の Story (#2056) で、デモの映像を流すための Vite のプラグイン。
// vite.config.ts と .storybook/main.ts の両方から読む。
import { readFileSync } from 'node:fs';

import type { Plugin } from 'vite-plus';

// mode が demo のときは、静的なデモ (#832) をビルドする。`vp build --mode demo` で使う。
// demoStream は、静的なデモ (#832) のときだけ、合成の映像 (demo/stream.m2ts) と、
// ライブ視聴と録画の再生で映像を繰り返して流す Service Worker (stream-sw.js と、それが読む demo/stream-loop.js) を出力に加える。
// public に置くと、tenban に埋め込む通常のビルドにも入るので、ここで加える。
// Service Worker は、デモの全体を受け持てるよう、base の直下に置く。
// Storybook の画面の Story (#2056) も、映像を流すのに使う (.storybook/main.ts)。
export function demoStream(): Plugin {
  const files = [
    { fileName: 'demo/stream.m2ts', source: './stream.m2ts', type: 'video/mp2t' },
    { fileName: 'stream-sw.js', source: './stream-sw.js', type: 'text/javascript' },
    { fileName: 'demo/stream-loop.js', source: './stream-loop.js', type: 'text/javascript' },
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
