// 字幕とデータ放送のフォント (vendor/caption-font の TTF) を、基本のファイルと、JIS X 0213 で足された漢字のファイルの 2 つに分け、
// WOFF2 にする (#1520、設計書 7.4)。後のファイルの字は、字幕や番組情報にめったに出ないので、出たときだけ読み込む。
// 後のファイルの字の範囲 (CSS の unicode-range の書き方) も、同じ所に書く。scripts/fetch-caption-font.mjs の後に動かす。
// 元の TTF は、デモの映像を作るスクリプトが使うので残す。
import { createHash } from 'node:crypto';
import { readFile, rename, writeFile } from 'node:fs/promises';
import { join } from 'node:path';

import fontverter from 'fontverter';
import * as hb from 'harfbuzzjs';
import subsetFont from 'subset-font';

const dir = join(import.meta.dirname, '..', 'vendor', 'caption-font');
const source = join(dir, 'rounded-mplus-1m-wadalab-mix.ttf');
const outputs = {
  base: join(dir, 'rounded-mplus-1m-wadalab-mix-base.woff2'),
  extended: join(dir, 'rounded-mplus-1m-wadalab-mix-jisx0213.woff2'),
  range: join(dir, 'rounded-mplus-1m-wadalab-mix-jisx0213.range.json'),
  stamp: join(dir, 'split.stamp'),
};

const font = await readFile(source);
const kanji = JSON.parse(await readFile(join(import.meta.dirname, 'jisx0213-kanji.json'), 'utf8'));

// 元のフォントと表が同じなら、作り直さない。
const stamp = createHash('sha256').update(font).update(JSON.stringify(kanji)).digest('hex');
if ((await readFile(outputs.stamp, 'utf8').catch(() => '')) === stamp) {
  process.exit(0);
}

const extendedSet = new Set();
for (const [start, end] of kanji.ranges) {
  for (let codepoint = start; codepoint <= end; codepoint++) {
    extendedSet.add(codepoint);
  }
}
const unicodesOf = (data) => [...new hb.Face(new hb.Blob(data)).collectUnicodes()];
const codepoints = unicodesOf(font);
const extended = codepoints.filter((codepoint) => extendedSet.has(codepoint)).sort((a, b) => a - b);
const base = codepoints.filter((codepoint) => !extendedSet.has(codepoint));

const toText = (list) => String.fromCodePoint(...list);
const [baseSfnt, extendedSfnt] = await Promise.all([
  subsetFont(font, toText(base), { targetFormat: 'sfnt' }),
  subsetFont(font, toText(extended), { targetFormat: 'sfnt' }),
]);

// 2 つのファイルの字が重ならず、合わせると元のフォントの字と同じになることを確かめる。外れたら、配らずに止める。
const baseCodepoints = unicodesOf(baseSfnt);
const extendedCodepoints = new Set(unicodesOf(extendedSfnt));
const overlap = baseCodepoints.filter((codepoint) => extendedCodepoints.has(codepoint));
const union = new Set([...baseCodepoints, ...extendedCodepoints]);
if (
  overlap.length > 0 ||
  union.size !== codepoints.length ||
  codepoints.some((codepoint) => !union.has(codepoint))
) {
  throw new Error(
    `the split fonts do not cover the original font exactly: ${overlap.length} overlapping, ${union.size} of ${codepoints.length} chars`,
  );
}
if (
  extendedCodepoints.size !== extended.length ||
  extended.some((codepoint) => !extendedCodepoints.has(codepoint))
) {
  throw new Error('the JIS X 0213 font does not match its unicode-range');
}
// WOFF2 への変換は、WASM の同じ領域を使うので、並べずに 1 つずつ行う。
const baseFont = await fontverter.convert(baseSfnt, 'woff2');
const extendedFont = await fontverter.convert(extendedSfnt, 'woff2');
for (const converted of [baseFont, extendedFont]) {
  if (fontverter.detectFormat(converted) !== 'woff2') {
    throw new Error('failed to convert the caption font to WOFF2');
  }
}

// unicode-range は、U+16進 か U+16進-16進 をカンマで並べる。連続する字は範囲にまとめる。
const ranges = [];
for (const codepoint of extended) {
  const last = ranges.at(-1);
  if (last !== undefined && last[1] === codepoint - 1) {
    last[1] = codepoint;
  } else {
    ranges.push([codepoint, codepoint]);
  }
}
const hex = (codepoint) => codepoint.toString(16).toUpperCase();
const unicodeRange = ranges
  .map(([start, end]) => (start === end ? `U+${hex(start)}` : `U+${hex(start)}-${hex(end)}`))
  .join(',');

// 途中で止まっても、壊れたファイルが残らないよう、書き終えてから置き換える。stamp は最後に書く。
const write = async (path, data) => {
  await writeFile(`${path}.tmp`, data);
  await rename(`${path}.tmp`, path);
};
await write(outputs.base, baseFont);
await write(outputs.extended, extendedFont);
await write(outputs.range, `${JSON.stringify({ unicodeRange })}\n`);
await write(outputs.stamp, stamp);
console.log(
  `split the caption font: ${base.length} chars (${baseFont.length} bytes) and ${extended.length} JIS X 0213 kanji (${extendedFont.length} bytes)`,
);
