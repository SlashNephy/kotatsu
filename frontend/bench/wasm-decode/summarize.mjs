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

const median = (xs) => {
  const s = [...xs].sort((a, b) => a - b);
  const m = Math.floor(s.length / 2);
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
};

// browserOf は、env の行の userAgent から、ブラウザの短い名前を返す。続く結果の行を、ブラウザと計測の手順ごとに分けるのに使う。
const browserOf = (userAgent) => {
  if (/Firefox\//.test(userAgent)) return 'Firefox';
  if (/Chrome\//.test(userAgent)) return 'Chrome';
  if (/Version\/.*Safari\//.test(userAgent)) return 'Safari';
  return 'other';
};
let browser = '';
const groups = new Map();
for (const row of rows) {
  if (row.kind === 'env') browser = `${browserOf(row.userAgent)}\t${row.plan ?? '-'}\t`;
  if (row.kind === 'env' || row.kind === 'done') {
    console.log(`${row.kind}:`, JSON.stringify(row));
    continue;
  }
  // WebGL のエラーが残った回は、アップロードが失敗して速く見えているおそれがあるので、失敗として外す。
  if (!row.ok || (row.glError ?? 0) !== 0) {
    console.log('failed:', JSON.stringify(row));
    continue;
  }
  // 小さなループ (loop.js) は、後ろの 3 回の中央値を出す。最初の回は最適化の前の段階で動くことがあるため。
  if (row.kind === 'loop') {
    const late = (xs) => median(xs.slice(-3)).toFixed(1);
    console.log(
      `${browser}loop\t${row.thread}\twasmMs=${late(row.wasmMs)}\tjsMs=${late(row.jsMs)}\tall wasm=${row.wasmMs.map((x) => x.toFixed(0)).join(',')} js=${row.jsMs.map((x) => x.toFixed(0)).join(',')}`,
    );
    continue;
  }
  const upload = row.upload && row.upload !== 'none' ? `\tupload=${row.upload}` : '';
  const thread = row.thread === 'main' ? '\tthread=main' : '';
  const key = `${browser}${row.file}\t${row.kind}\t${row.variant ?? 'mpeg2toh264'}${upload}${thread}`;
  const values =
    row.kind === 'decode'
      ? {
          fps: row.fps,
          videoFps: row.videoFps,
          avgMs: row.avgMs,
          p50: row.p50,
          p95: row.p95,
          p99: row.p99,
          firstMs: row.firstFrameMs,
          frames: row.frames,
          // アップロードを足した回だけ (#2162)。デコードだけの時間は、PES ごとの時間からアップロードを差し引いたもの
          ...(row.uploadAvgMs === undefined
            ? {}
            : {
                decodeAvgMs: row.decodeOnlyAvgMs,
                decodeP50: row.decodeOnlyP50,
                uploadAvgMs: row.uploadAvgMs,
                uploadP95: row.uploadP95,
                flushAvgMs: row.flushAvgMs,
              }),
        }
      : {
          fps: row.fps,
          firstMs: row.firstFragmentMs,
          mbps: (row.bytes * 8) / 30 / 1e6,
          frames: row.stats?.videoFrames,
        };
  if (!groups.has(key)) groups.set(key, []);
  groups.get(key).push(values);
}

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
