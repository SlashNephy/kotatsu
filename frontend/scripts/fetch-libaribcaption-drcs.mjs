// 字幕の外字を文字に置き換える既定のテーブルを、vendor/libaribcaption/SOURCE で固定した commit の libaribcaption の定義から作り、
// vendor/libaribcaption/drcs.json に置く。サーバーも字幕の文を取り出すときに使うので (#2216)、同じものを
// ../backend/internal/lib/drcstable/dist/drcs.json にも置き、本体に埋め込む。
// 同じ commit から作ったテーブルが両方に既にあれば、取得し直さない。
// 本番のイメージのビルド (Dockerfile) でも動くよう、Node.js の標準の機能だけを使う。
import { copyFile, mkdir, readFile, rename, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';

const dir = join(import.meta.dirname, '..', 'vendor', 'libaribcaption');
const source = Object.fromEntries(
  (await readFile(join(dir, 'SOURCE'), 'utf8'))
    .split('\n')
    .filter((line) => line !== '' && !line.startsWith('#'))
    .map((line) => line.split('=', 2)),
);
for (const key of ['repository', 'file', 'commit']) {
  if (source[key] === undefined) {
    throw new Error(`${key} is missing in vendor/libaribcaption/SOURCE`);
  }
}

const target = join(dir, 'drcs.json');
const backendTarget = join(import.meta.dirname, '..', '..', 'backend/internal/lib/drcstable/dist/drcs.json');
// 本番のイメージのビルドでは、SPA のステージに backend/ がないので、置き場所から作る。
await mkdir(dirname(backendTarget), { recursive: true });
// 途中で壊れたテーブルは、作っていないものとして作り直す。
const generatedFrom = async (path) => {
  const existing = await readFile(path, 'utf8').catch(() => null);
  try {
    return existing === null ? null : JSON.parse(existing).commit;
  } catch {
    return null;
  }
};
if ((await generatedFrom(target)) === source.commit) {
  if ((await generatedFrom(backendTarget)) !== source.commit) {
    await copyFile(target, `${backendTarget}.tmp`);
    await rename(`${backendTarget}.tmp`, backendTarget);
  }
  process.exit(0);
}

// 上流の書き方が変わってテーブルが空に近くなったら、黙って外字を置き換えなくなるのではなく、ビルドを失敗させる。
const minimumEntries = 100;

const url = `https://raw.githubusercontent.com/${source.repository}/${source.commit}/${source.file}`;
const response = await fetch(url);
if (!response.ok) {
  throw new Error(`failed to download ${url}: ${response.status} ${response.statusText}`);
}
const entries = [...(await response.text()).matchAll(/\{"([0-9a-f]{32})",\s*0x([0-9a-fA-F]+)\}/g)]
  .map(([, md5, codePoint]) => [md5, String.fromCodePoint(Number.parseInt(codePoint, 16))])
  .sort(([a], [b]) => (a < b ? -1 : 1));
const replacements = Object.fromEntries(entries);
const count = Object.keys(replacements).length;
if (count < minimumEntries) {
  throw new Error(`only ${count} DRCS replacements were found in ${url}`);
}
// 途中で止まっても壊れたテーブルが残らないよう、書き終えてから置き換える。
await writeFile(`${target}.tmp`, `${JSON.stringify({ commit: source.commit, replacements })}\n`);
await rename(`${target}.tmp`, target);
await copyFile(target, `${backendTarget}.tmp`);
await rename(`${backendTarget}.tmp`, backendTarget);
console.log(`generated ${count} DRCS replacements (${source.commit})`);
