// 測定のページ。ボタンからも、ブラウザの自動操作 (window.runDecode、window.runConvert) からも測れる。
const log = document.getElementById('log');
// append は、結果を画面に出し、配信しているサーバーにも送る (serve.mjs の POST /results)。実機の画面から結果をコピーしなくても済むように。
// 送るのは 1 行ずつ順に行い、行の順が入れ替わらないようにする。sending は送り終わりを待つ Promise、sendFailed は送れなかった行の数。
let sending = Promise.resolve();
let sendFailed = 0;
const append = (result) => {
  const line = JSON.stringify({ ...result, sums: undefined });
  log.value += `${line}\n`;
  log.scrollTop = log.scrollHeight;
  sending = sending.then(async () => {
    try {
      const response = await fetch('./results', { method: 'POST', body: line });
      if (!response.ok) sendFailed++;
    } catch {
      sendFailed++;
    }
  });
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

/** 小さなループの計測 (loop.js) を、worker で 1 回行う。 */
const runLoopInWorker = () =>
  new Promise((resolve) => {
    const worker = new Worker('./decode-worker.js', { type: 'module' });
    worker.onmessage = ({ data }) => {
      worker.terminate();
      append(data);
      resolve(data);
    };
    worker.onerror = (event) => {
      worker.terminate();
      const data = { ok: false, kind: 'loop', thread: 'worker', error: event.message };
      append(data);
      resolve(data);
    };
    worker.postMessage({ kind: 'loop' });
  });

/** 小さなループの計測 (loop.js) を、メインスレッドで 1 回行う。 */
const runLoopInMain = async () => {
  const { measureLoop } = await import('./loop.js');
  const data = await measureLoop();
  append(data);
  return data;
};

/** デコードの計測 (decode.js) を、メインスレッドで 1 回行う。計測の間は画面が止まる。 */
const runDecodeInMain = async (file, variant, options) => {
  const { measureDecode } = await import('./decode.js');
  const data = await measureDecode({ file, variant, ...options });
  append(data);
  return data;
};

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

// #2162 の計測の手順。どれも 1 回を 300 コマ (10 秒分) で打ち切り、遅い端末でも数分で終わるようにする。
// split は、製品の WASM で、アップロードなしと 3 通りの上げ方を、映像 2 つで交互に 2 回ずつ計測する。
// variants は、WASM の組み方 (build.sh の VARIANTS と製品) を、アップロードなしで交互に 2 回ずつ計測する。
// jit は、小さなループ (WASM と JavaScript) と製品の WASM のデコードを、worker とメインスレッドで交互に 2 回ずつ計測する。
// 手順の 1 つは、画面に出す名前 (label) と、計測して結果を返す関数 (run) の組。
const decodeStep = (file, variant, upload) => ({
  label: `${file}、${variant}、${upload}`,
  run: () => runDecode(file, variant, false, { upload, maxFrames: 300 }),
});
const plans = {
  split: () =>
    [0, 1].flatMap(() =>
      ['mandel_1440.ts', 'testsrc2_1440.ts'].flatMap((file) =>
        ['none', 'offset', 'subarray', 'copy'].map((upload) => decodeStep(file, 'product', upload)),
      ),
    ),
  variants: () =>
    [0, 1].flatMap(() =>
      ['product', 'O3', 'O3-simd', 'O3-lto', 'O3-simd-lto', 'O2-simd-lto', 'Os-simd-lto'].map(
        (variant) => decodeStep('mandel_1440.ts', variant, 'none'),
      ),
    ),
  jit: () =>
    [0, 1].flatMap(() => [
      { label: '小さなループ、worker', run: runLoopInWorker },
      { label: '小さなループ、メインスレッド', run: runLoopInMain },
      decodeStep('mandel_1440.ts', 'product', 'none'),
      {
        label: 'mandel_1440.ts、product、メインスレッド',
        run: () => runDecodeInMain('mandel_1440.ts', 'product', { upload: 'none', maxFrames: 300 }),
      },
    ]),
};
const status = document.getElementById('split-status');
const runPlan = async (plan) => {
  if (!Object.hasOwn(plans, plan)) throw new Error(`unknown plan: ${plan}`);
  const buttons = document.querySelectorAll('button[data-plan]');
  for (const button of buttons) button.disabled = true;
  log.value = '';
  sendFailed = 0;
  append({
    kind: 'env',
    plan,
    userAgent: navigator.userAgent,
    crossOriginIsolated: self.crossOriginIsolated ?? null,
    hardwareConcurrency: navigator.hardwareConcurrency ?? null,
  });
  const steps = plans[plan]();
  const total = steps.length;
  let done = 0;
  // failed は、失敗した回と、WebGL のエラーが残った回の数。
  let failed = 0;
  try {
    for (const step of steps) {
      status.textContent = `計測中 ${done + 1} / ${total}（${step.label}）`;
      // メインスレッドで計測する手順の前に、画面が書き換わる間を置く。
      await new Promise((resolve) => setTimeout(resolve, 100));
      const result = await step.run();
      if (!result.ok || (result.glError ?? 0) !== 0) failed++;
      done++;
    }
    // 送るのは順なので、終わりの行は計測の行の後に届く。終わりの行を見て集計を始めても、行が欠けない。
    append({ kind: 'done', plan, total, failed });
    await sending;
    const counted =
      failed === 0
        ? `終わりました（${total} 回）。`
        : `終わりました（${total} 回のうち ${failed} 回は失敗か WebGL のエラー）。`;
    status.textContent =
      sendFailed === 0
        ? `${counted}結果はサーバーに送りました。`
        : `${counted}サーバーに送れなかった結果が ${sendFailed} 行あります。「結果をコピー」でコピーしてください。`;
  } catch (error) {
    append({ kind: 'done', plan, total, failed, error: String(error) });
    await sending;
    status.textContent = `失敗しました: ${String(error)}`;
  } finally {
    for (const button of buttons) button.disabled = false;
  }
};
window.runPlan = runPlan;
for (const button of document.querySelectorAll('button[data-plan]')) {
  button.onclick = () => runPlan(button.dataset.plan);
}
// URL に ?split、?variants、?jit のどれかを付けて開いたら、ボタンを押さなくてもその計測を始める。音は鳴らさないので、操作がなくても動く。
const query = new URLSearchParams(location.search);
const planFromUrl = Object.keys(plans).find((plan) => query.has(plan));
if (planFromUrl) void runPlan(planFromUrl);
document.getElementById('copy').onclick = async () => {
  try {
    await navigator.clipboard.writeText(log.value);
    status.textContent = 'コピーしました。';
  } catch {
    log.select();
    status.textContent = 'コピーできませんでした。結果の欄を選んだので、⌘C でコピーしてください。';
  }
};
