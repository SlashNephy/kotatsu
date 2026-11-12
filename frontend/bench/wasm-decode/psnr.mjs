// mpeg2toh264 の出力と元の MPEG-2 の輝度の PSNR を、コマのずれを探して出す。
//
// 出力には同じ絵のコマが重なって入り、PTS も重なるので、PTS では合わせられない。
// 出力を k コマ (0..MAX_SHIFT) ずらして元と並べた PSNR を ffmpeg で出し、元のコマごとに最も高い k の値を取る。
// 輝度の面だけを取り出して比べる。そのまま psnr に渡すと、色の範囲の違いで片方が変換され、値が大きく下がる。
// 使い方: node psnr.mjs 出力.mp4 元.ts
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, join } from 'node:path';

const MAX_SHIFT = 12;
const [out, src] = process.argv.slice(2);
if (!out || !src) {
  throw new Error('usage: node psnr.mjs <out.mp4> <src.ts>');
}

const tmp = mkdtempSync(join(tmpdir(), 'psnr-'));
try {
  const runs = [];
  for (let k = 0; k <= MAX_SHIFT; k++) {
    const log = join(tmp, `${k}.log`);
    const graph =
      `[0:v]trim=start_frame=${k},extractplanes=y,setpts=N/(30000/1001)/TB[a];` +
      '[1:v]extractplanes=y,setpts=N/(30000/1001)/TB[b];' +
      `[a][b]psnr=stats_file=${log}:shortest=1`;
    execFileSync(
      'ffmpeg',
      ['-v', 'error', '-i', out, '-i', src, '-filter_complex', graph, '-f', 'null', '-'],
      {
        stdio: 'inherit',
      },
    );
    runs.push(
      readFileSync(log, 'utf8')
        .split('\n')
        .filter((line) => line !== '')
        .map((line) => Math.min(99, Number(/psnr_y:(\S+)/.exec(line)?.[1] ?? 0))),
    );
  }
  const n = Math.max(...runs.map((r) => r.length));
  const best = [];
  for (let i = 0; i < n; i++) {
    best.push(Math.max(...runs.filter((r) => i < r.length).map((r) => r[i] ?? 0)));
  }
  best.sort((a, b) => a - b);
  const at = (q) => best[Math.min(best.length - 1, Math.floor(q * best.length))] ?? Number.NaN;
  console.log(
    `${basename(out)} compared=${best.length} median_y=${at(0.5).toFixed(2)} p5=${at(0.05).toFixed(2)} min=${(best[0] ?? Number.NaN).toFixed(2)}`,
  );
} finally {
  rmSync(tmp, { recursive: true, force: true });
}
