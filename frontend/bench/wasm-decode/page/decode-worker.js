// WASM の FFmpeg でそのままデコードする方式の測定。読み込みは測定から外す (先に全部メモリに置く)。描画はしない。
import { demux } from './ts.js';

const quantile = (values, q) => {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.min(sorted.length - 1, Math.floor(q * sorted.length))];
};

// AV_FRAME_FLAG_INTERLACED
const FRAME_FLAG_INTERLACED = 8;

onmessage = async ({ data: { file, variant, checksum } }) => {
  try {
    const { default: init } = await import(`./wasm/${variant}/dec.mjs`);
    const t0 = performance.now();
    const M = await init({ locateFile: (path) => `./wasm/${variant}/${path}` });
    const initMs = performance.now() - t0;
    const response = await fetch(`./media/${file}`);
    if (!response.ok) throw new Error(`failed to fetch ${file}: ${response.status}`);
    const buf = await response.arrayBuffer();
    const td = performance.now();
    const { video, audio, ccErrors } = demux(buf);
    const demuxMs = performance.now() - td;

    const vd = M._dec_open(0);
    const ad = M._dec_open(1);
    const outp = M._malloc(4);
    let cap = 1 << 20;
    let inp = M._malloc(cap);
    const ensure = (n) => {
      if (n > cap) {
        M._free(inp);
        cap = n * 2;
        inp = M._malloc(cap);
      }
    };
    const times = [];
    const sums = [];
    let frames = 0;
    let interlaced = 0;
    let firstFrameMs = -1;
    const vStart = performance.now();
    const drain = () => {
      while (M._dec_receive(vd) === 0) {
        frames++;
        if (firstFrameMs < 0) firstFrameMs = performance.now() - vStart;
        if (M._frame_flags(vd) & FRAME_FLAG_INTERLACED) interlaced++;
        if (checksum && frames <= 60) {
          // 先頭の Y の和で、ネイティブの ffmpeg と同じ絵が出ているかを見る
          const p = M._frame_data(vd, 0);
          const ls = M._frame_linesize(vd, 0);
          const w = M._frame_width(vd);
          const h = M._frame_height(vd);
          let s = 0;
          for (let y = 0; y < h; y++) {
            const row = M.HEAPU8.subarray(p + y * ls, p + y * ls + w);
            for (let x = 0; x < w; x++) s += row[x];
          }
          sums.push(s);
        }
      }
    };
    const parseAndDecode = (ptr, size, pts) => {
      let off = 0;
      do {
        off += M._dec_parse(vd, ptr + off, size - off, off === 0 ? pts : -1, outp);
        if (M.HEAP32[outp >> 2]) {
          M._dec_send_parsed(vd);
          drain();
        }
      } while (off < size);
    };
    for (const pes of video) {
      const t = performance.now();
      ensure(pes.data.length);
      M.HEAPU8.set(pes.data, inp);
      parseAndDecode(inp, pes.data.length, pes.pts);
      times.push(performance.now() - t);
    }
    // parser に残った最後のフレームと、並べ替えで残ったフレームを出す
    parseAndDecode(0, 0, -1);
    M._dec_send(vd, 0, 0, -1);
    drain();
    const videoMs = performance.now() - vStart;

    const aStart = performance.now();
    let samples = 0;
    let audioErrors = 0;
    // ADTS の区切りは JS で付け、ヘッダの解釈はデコーダに任せる
    for (const pes of audio) {
      const d = pes.data;
      for (let o = 0; o + 7 <= d.length;) {
        if (d[o] !== 0xff || (d[o + 1] & 0xf0) !== 0xf0) {
          o++;
          continue;
        }
        const len = ((d[o + 3] & 3) << 11) | (d[o + 4] << 3) | (d[o + 5] >> 5);
        if (len < 7 || o + len > d.length) break;
        ensure(len);
        M.HEAPU8.set(d.subarray(o, o + len), inp);
        if (M._dec_send(ad, inp, len, -1) < 0) audioErrors++;
        while (M._dec_receive(ad) === 0) samples += M._frame_nb_samples(ad);
        o += len;
      }
    }
    M._dec_send(ad, 0, 0, -1);
    while (M._dec_receive(ad) === 0) samples += M._frame_nb_samples(ad);
    const audioMs = performance.now() - aStart;
    const totalMs = demuxMs + videoMs + audioMs;
    postMessage({
      ok: true,
      kind: 'decode',
      file,
      variant,
      initMs,
      demuxMs,
      ccErrors,
      videoPes: video.length,
      frames,
      interlaced,
      firstFrameMs,
      videoMs,
      audioMs,
      // TS → PES と音のデコードまで含めた速さ。変換の方式も TS の読み解きと音を含むので、比べるのはこちら
      fps: frames / (totalMs / 1000),
      videoFps: frames / (videoMs / 1000),
      avgMs: times.reduce((a, b) => a + b, 0) / times.length,
      p50: quantile(times, 0.5),
      p95: quantile(times, 0.95),
      p99: quantile(times, 0.99),
      max: Math.max(...times),
      samples,
      audioErrors,
      sums,
    });
  } catch (error) {
    postMessage({ ok: false, kind: 'decode', file, variant, error: String(error?.stack ?? error) });
  }
};
