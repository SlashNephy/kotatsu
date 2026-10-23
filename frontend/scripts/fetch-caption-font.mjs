// 字幕を描くフォントを、vendor/caption-font/SOURCE で固定したリリースから取得し、vendor/caption-font に置く。
// sha256 が合わなければ失敗する。同じ sha256 のフォントが既にあれば、取得し直さない。
// 本番のイメージのビルド (Dockerfile) でも動くよう、Node.js の標準の機能だけを使う。
import { createHash } from 'node:crypto';
import { readFile, rename, writeFile } from 'node:fs/promises';
import { join } from 'node:path';

const dir = join(import.meta.dirname, '..', 'vendor', 'caption-font');
const source = Object.fromEntries(
  (await readFile(join(dir, 'SOURCE'), 'utf8'))
    .split('\n')
    .filter((line) => line !== '' && !line.startsWith('#'))
    .map((line) => line.split('=', 2)),
);
for (const key of ['repository', 'file', 'tag', 'sha256']) {
  if (source[key] === undefined) {
    throw new Error(`${key} is missing in vendor/caption-font/SOURCE`);
  }
}

const sha256 = (data) => createHash('sha256').update(data).digest('hex');
const target = join(dir, source.file);
const existing = await readFile(target).catch(() => null);
if (existing !== null && sha256(existing) === source.sha256) {
  process.exit(0);
}

const url = `https://github.com/${source.repository}/releases/download/${source.tag}/${source.file}`;
const response = await fetch(url);
if (!response.ok) {
  throw new Error(`failed to download ${url}: ${response.status} ${response.statusText}`);
}
const data = Buffer.from(await response.arrayBuffer());
const actual = sha256(data);
if (actual !== source.sha256) {
  throw new Error(`sha256 mismatch for ${url}: got ${actual}, want ${source.sha256}`);
}
// 途中で止まっても壊れたフォントが残らないよう、書き終えてから置き換える。
await writeFile(`${target}.tmp`, data);
await rename(`${target}.tmp`, target);
console.log(`fetched ${source.file} (${source.tag})`);
