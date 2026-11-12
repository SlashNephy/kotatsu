// 測定のページの結果 (1 行に 1 つの JSON) を、映像と方式ごとに、中央値 (最小〜最大) でまとめる。
// 使い方: node summarize.mjs 結果.jsonl
import { readFileSync } from 'node:fs';

const [path] = process.argv.slice(2);
if (!path) {
  throw new Error('usage: node summarize.mjs <results.jsonl>');
}
const rows = readFileSync(path, 'utf8')
  .split('\n')
  .filter((line) => line.trim() !== '')
  .map((line) => JSON.parse(line));

const groups = new Map();
for (const row of rows) {
  if (!row.ok) {
    console.log('failed:', JSON.stringify(row));
    continue;
  }
  const key = `${row.file}\t${row.kind}\t${row.variant ?? 'mpeg2toh264'}`;
  const values =
    row.kind === 'decode'
      ? { fps: row.fps, p95: row.p95, p99: row.p99, firstMs: row.firstFrameMs, frames: row.frames }
      : {
          fps: row.fps,
          firstMs: row.firstFragmentMs,
          mbps: (row.bytes * 8) / 30 / 1e6,
          frames: row.stats?.videoFrames,
        };
  if (!groups.has(key)) groups.set(key, []);
  groups.get(key).push(values);
}

const median = (xs) => {
  const s = [...xs].sort((a, b) => a - b);
  const m = Math.floor(s.length / 2);
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
};
const format = (xs) => {
  const v = xs.filter((x) => typeof x === 'number' && Number.isFinite(x));
  if (v.length === 0) return '-';
  return `${median(v).toFixed(1)} (${Math.min(...v).toFixed(1)}–${Math.max(...v).toFixed(1)})`;
};
for (const key of [...groups.keys()].sort((a, b) => a.localeCompare(b))) {
  const runs = groups.get(key);
  const columns = Object.keys(runs[0]).map((name) => `${name}=${format(runs.map((r) => r[name]))}`);
  console.log(`${key}\tn=${runs.length}\t${columns.join('\t')}`);
}
