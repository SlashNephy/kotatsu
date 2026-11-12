// 測定のページを配る静的なサーバ。実機からも開けるよう、待ち受ける場所は引数で変えられる。
//
//   /          page/ (測定のページ)
//   /wasm/     .work/out/ (build.sh で組んだ WASM)
//   /media/    .work/media/ (make-media.sh で作った TS)。Range に応える
//   /m2h/      mpeg2toh264 の worker (/m2h/worker.js) と WASM (public/mpeg2toh264)
//   PUT /save/<名前>  mpeg2toh264 の出力を .work/save/ に書く (PSNR とコマ数を測るため)
//
// 使い方: node serve.mjs [ポート] [待ち受けるアドレス]
import { createReadStream, createWriteStream } from 'node:fs';
import { mkdir, readdir, stat } from 'node:fs/promises';
import { createServer } from 'node:http';
import { extname, join, normalize, resolve, sep } from 'node:path';
import { pipeline } from 'node:stream/promises';

const here = import.meta.dirname;
const frontend = resolve(here, '..', '..');
const port = Number(process.argv[2] ?? 18516);
const host = process.argv[3] ?? '127.0.0.1';

const mounts = [
  ['/wasm/', join(here, '.work', 'out')],
  ['/media/', join(here, '.work', 'media')],
  ['/m2h/', join(frontend, 'public', 'mpeg2toh264')],
  ['/', join(here, 'page')],
];
const types = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript',
  '.mjs': 'text/javascript',
  '.wasm': 'application/wasm',
  '.ts': 'video/mp2t',
  '.json': 'application/json',
};

// mpeg2toh264 の worker のファイル名は、ビルドのたびに変わる。/m2h/worker.js で、そのときの worker を返す。
const m2hWorker = async () => {
  const dir = join(frontend, 'vendor', 'mpeg2toh264', 'player', 'dist', 'assets');
  const name = (await readdir(dir)).find((entry) => /^worker-.*\.js$/.test(entry));
  return name ? join(dir, name) : null;
};

const locate = async (pathname) => {
  if (pathname === '/m2h/worker.js') return m2hWorker();
  for (const [prefix, dir] of mounts) {
    if (pathname.startsWith(prefix)) {
      const path = normalize(join(dir, pathname.slice(prefix.length) || 'index.html'));
      return path === dir || path.startsWith(dir + sep) ? path : null;
    }
  }
  return null;
};

createServer(async (req, res) => {
  res.setHeader('Cache-Control', 'no-store');
  try {
    const { pathname } = new URL(req.url ?? '/', 'http://localhost');
    const save = /^\/save\/([\w.-]+)$/.exec(pathname);
    if (req.method === 'PUT' && save?.[1]) {
      const dir = join(here, '.work', 'save');
      await mkdir(dir, { recursive: true });
      await pipeline(req, createWriteStream(join(dir, save[1])));
      res.writeHead(201).end();
      return;
    }
    const path = await locate(decodeURIComponent(pathname));
    const info = path ? await stat(path).catch(() => null) : null;
    if (!path || !info?.isFile()) {
      res.writeHead(404).end();
      return;
    }
    const headers = {
      'Content-Type': types[extname(path)] ?? 'application/octet-stream',
      'Accept-Ranges': 'bytes',
    };
    const range = /^bytes=(\d*)-(\d*)$/.exec(req.headers.range ?? '');
    if (range) {
      const [, first, last] = range;
      const start = first === '' ? info.size - Number(last) : Number(first);
      const end =
        first !== '' && last !== '' ? Math.min(Number(last), info.size - 1) : info.size - 1;
      res.writeHead(206, {
        ...headers,
        'Content-Range': `bytes ${start}-${end}/${info.size}`,
        'Content-Length': end - start + 1,
      });
      await pipeline(createReadStream(path, { start, end }), res);
      return;
    }
    res.writeHead(200, { ...headers, 'Content-Length': info.size });
    await pipeline(createReadStream(path), res);
  } catch (error) {
    console.error(error);
    if (!res.headersSent) res.writeHead(500);
    res.end();
  }
}).listen(port, host, () => console.log(`serving on http://${host}:${port}/`));
