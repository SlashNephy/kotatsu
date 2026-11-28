// 測定のページ。ボタンからも、ブラウザの自動操作 (window.runDecode、window.runConvert) からも測れる。
const log = document.getElementById('log');
const append = (result) => {
  log.value += `${JSON.stringify({ ...result, sums: undefined })}\n`;
  log.scrollTop = log.scrollHeight;
};

/**
 * WASM でそのままデコードする方式を 1 回計測する。checksum を付けると、先頭 60 コマの輝度の和も返す。
 * upload に上げ方 (offset、subarray、copy) を渡すと、コマを WebGL2 のテクスチャへ上げ、デコードと分けて時間を返す (#2162)。
 * maxFrames を渡すと、そのコマ数をデコードしたところで映像を打ち切る。
 */
const runDecode = (file, variant, checksum = false, { upload = 'none', maxFrames } = {}) =>
  new Promise((resolve) => {
    const worker = new Worker('./decode-worker.js', { type: 'module' });
    worker.onmessage = ({ data }) => {
      worker.terminate();
      append(data);
      resolve(data);
    };
    worker.onerror = (event) => {
      worker.terminate();
      const data = { ok: false, kind: 'decode', file, variant, upload, error: event.message };
      append(data);
      resolve(data);
    };
    worker.postMessage({ file, variant, checksum, upload, maxFrames });
  });

/**
 * mpeg2toh264 の変換だけを 1 回計測する。sink を main にし、断片は MSE に入れずに受け取るだけにする。
 * オプションは Kotatsu と同じ既定値で、比較の条件を揃えるため pictureWorkers だけ 1 にする。
 * save を付けると、出力の MP4 をサーバの .work/save/ に書く。
 */
const runConvert = (file, { pictureWorkers = 1, save = false } = {}) =>
  new Promise((resolve) => {
    const worker = new Worker('./m2h/worker.js', { type: 'module' });
    const id = 1;
    const marks = {};
    const parts = [];
    // stats の convertingMs などは、前の報告からの差分で届くので足し上げる
    const sum = { convertingMs: 0, readingMs: 0, waitingMs: 0 };
    let last = null;
    let bytes = 0;
    let mime = null;
    let workers = 1;
    const t0 = performance.now();
    worker.onerror = (event) => {
      worker.terminate();
      const result = { ok: false, kind: 'convert', file, error: event.message };
      append(result);
      resolve(result);
    };
    worker.onmessage = async ({ data: m }) => {
      switch (m.type) {
        case 'mark':
          marks[m.name] = performance.now() - t0;
          return;
        case 'open':
        case 'fragment':
          if (m.type === 'open') mime = m.mimeCodec;
          bytes += m.data.byteLength;
          if (save) parts.push(m.data);
          worker.postMessage({ type: 'flow', id, ready: true });
          return;
        case 'stats':
          last = m.stats;
          sum.convertingMs += m.stats.convertingMs;
          sum.readingMs += m.stats.readingMs;
          sum.waitingMs += m.stats.waitingMs;
          return;
        case 'workers':
          workers = m.pictureWorkers;
          return;
        case 'completed':
        case 'error': {
          const totalMs = performance.now() - t0;
          worker.terminate();
          let saved = null;
          if (save) {
            const response = await fetch(`./save/${file}.mp4`, {
              method: 'PUT',
              body: new Blob(parts),
            });
            saved = response.status;
          }
          const result = {
            ok: m.type === 'completed',
            kind: 'convert',
            file,
            error: m.message,
            pictureWorkers: workers,
            mime,
            bytes,
            totalMs,
            firstFragmentMs: marks['first-fragment'] ?? null,
            marks,
            stats: last,
            sum,
            // 変換の時間だけで割った速さ。読み込みと受け渡しの待ちは含めない
            fps: last ? last.videoFrames / (sum.convertingMs / 1000) : null,
            saved,
          };
          append(result);
          resolve(result);
          return;
        }
        default:
      }
    };
    worker.postMessage({
      type: 'load',
      id,
      url: new URL(`./media/${file}`, location.href).href,
      wasmUrl: new URL('./m2h/mpeg2toh264_wasm_bg.wasm', location.href).href,
      oversample: undefined,
      recoveryInterval: undefined,
      openGopRecovery: undefined,
      splitFieldSamples: undefined,
      passthrough: false,
      pictureWorkers,
      serviceId: null,
      sink: 'main',
      preferManagedMediaSource: false,
      queueHighWaterMark: 33554432,
      maxAheadSeconds: 8,
      keepBehindSeconds: 10,
    });
  });

window.runDecode = runDecode;
window.runConvert = runConvert;

const words = (id) => document.getElementById(id).value.split(/\s+/).filter(Boolean);
const run = async ({ decode, convert }) => {
  const repeat = Number(document.getElementById('repeat').value) || 1;
  for (const file of words('files')) {
    for (let i = 0; i < repeat; i++) {
      if (decode) for (const variant of words('variants')) await runDecode(file, variant);
      if (convert) await runConvert(file);
    }
  }
};
document.getElementById('run-decode').onclick = () => run({ decode: true, convert: false });
document.getElementById('run-convert').onclick = () => run({ decode: false, convert: true });
document.getElementById('run-all').onclick = () => run({ decode: true, convert: true });

// #2162 の切り分け。製品の WASM で、アップロードなしと 3 通りの上げ方を、交互に 2 回ずつ計測する。
// 各回は 300 コマ (10 秒分) で打ち切り、遅い端末でも数分で終わるようにする。
const splitFiles = ['mandel_1440.ts', 'testsrc2_1440.ts'];
const splitModes = ['none', 'offset', 'subarray', 'copy'];
const status = document.getElementById('split-status');
const runSplit = async () => {
  const button = document.getElementById('run-split');
  button.disabled = true;
  log.value = '';
  log.value += `${JSON.stringify({ kind: 'env', userAgent: navigator.userAgent, crossOriginIsolated: self.crossOriginIsolated ?? null, hardwareConcurrency: navigator.hardwareConcurrency ?? null })}\n`;
  const total = splitFiles.length * splitModes.length * 2;
  let done = 0;
  try {
    for (let round = 0; round < 2; round++) {
      for (const file of splitFiles) {
        for (const upload of splitModes) {
          status.textContent = `計測中 ${done + 1} / ${total}（${file}、${upload}）`;
          await runDecode(file, 'product', false, { upload, maxFrames: 300 });
          done++;
        }
      }
    }
    status.textContent = `終わりました（${total} 回）。結果をコピーしてください。`;
  } catch (error) {
    status.textContent = `失敗しました: ${String(error)}`;
  } finally {
    button.disabled = false;
  }
};
window.runSplit = runSplit;
document.getElementById('run-split').onclick = runSplit;
document.getElementById('copy').onclick = async () => {
  try {
    await navigator.clipboard.writeText(log.value);
    status.textContent = 'コピーしました。';
  } catch {
    log.select();
    status.textContent = 'コピーできませんでした。結果の欄を選んだので、⌘C でコピーしてください。';
  }
};
