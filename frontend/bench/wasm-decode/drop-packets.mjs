// TS のパケットを割合で落とす。PAT と、ffmpeg の mpegts が使う PMT (PID 0x1000) は残す。
// 同じ種からは同じパケットを落とすよう、乱数は種から作る。
// 使い方: node drop-packets.mjs 元.ts 出力.ts 割合 種
import { readFile, writeFile } from 'node:fs/promises';

const [src, dst, rateArg, seedArg] = process.argv.slice(2);
if (!src || !dst || !rateArg || !seedArg) {
  throw new Error('usage: node drop-packets.mjs <in.ts> <out.ts> <rate> <seed>');
}
const rate = Number(rateArg);
// mulberry32
let state = Number(seedArg) >>> 0;
const random = () => {
  state = (state + 0x6d2b79f5) >>> 0;
  let t = state;
  t = Math.imul(t ^ (t >>> 15), t | 1);
  t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
};

const input = await readFile(src);
const output = Buffer.alloc(input.length);
let written = 0;
let dropped = 0;
const total = Math.floor(input.length / 188);
for (let i = 0; i < total; i++) {
  const packet = input.subarray(i * 188, i * 188 + 188);
  const pid = ((packet[1] & 0x1f) << 8) | packet[2];
  if (pid >= 0x20 && pid !== 0x1000 && random() < rate) {
    dropped++;
    continue;
  }
  packet.copy(output, written);
  written += 188;
}
await writeFile(dst, output.subarray(0, written));
console.log(`${dst} dropped=${dropped}/${total}`);
