// 実時間で再生する測定のメインスレッドの側 (#1516)。音を Web Audio で隙間なく予約して鳴らし、
// 「いま耳に届いている PTS」と画面の切り替わりの時刻を worker へ送る。描くのは worker の OffscreenCanvas。
// 音は鳴らすが、測定の間に音を出さないよう、ゲインを 0 にする (時計は、鳴らした位置で進む)。

// 鳴らし始めの余裕 (秒)、時計を送る間隔 (ms)
const startLead = 0.3;
const clockIntervalMs = 8;
// PTS の 1 秒 (90kHz)
const SECOND = 90_000;

/**
 * file を、組み方 variant の WASM でデコードし、seconds 秒だけ実時間で再生して測る。
 * canvas は、描く先の canvas 要素 (一度しか worker へ渡せないので、呼ぶたびに作り直す)。
 */
export const runPlay = (file, variant, { seconds = 25, width = 1920, height = 1080 } = {}) =>
  new Promise((resolve) => {
    const host = document.getElementById('stage');
    host.replaceChildren();
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    canvas.style.width = '100%';
    host.append(canvas);
    const offscreen = canvas.transferControlToOffscreen();

    const ctx = new AudioContext({ latencyHint: 'playback', sampleRate: 48_000 });
    const gain = ctx.createGain();
    gain.gain.value = 0;
    gain.connect(ctx.destination);
    let p0 = null;
    let t0 = null;
    let lateAudio = 0;
    let audioChunks = 0;
    let raf = 0;
    let clockTimer = 0;

    const worker = new Worker('./play-worker.js', { type: 'module' });
    const stop = (result) => {
      cancelAnimationFrame(raf);
      clearInterval(clockTimer);
      worker.terminate();
      // 閉じられなくても、測った値は返す
      ctx.close().catch(() => {});
      const out = {
        ...result,
        audio: {
          chunks: audioChunks,
          late: lateAudio,
          baseLatency: ctx.baseLatency,
          outputLatency: ctx.outputLatency,
        },
      };
      resolve(out);
    };
    worker.onerror = (event) =>
      stop({ ok: false, kind: 'play', file, variant, error: event.message });
    worker.onmessage = ({ data }) => {
      switch (data.type) {
        case 'audio': {
          audioChunks++;
          if (p0 === null) {
            p0 = data.pts;
            t0 = ctx.currentTime + startLead;
          }
          const at = t0 + (data.pts - p0) / SECOND;
          if (at < ctx.currentTime + 0.005) {
            // 番を過ぎた音は捨てる
            lateAudio++;
            return;
          }
          const buffer = ctx.createBuffer(2, data.left.length, 48_000);
          buffer.copyToChannel(data.left, 0);
          buffer.copyToChannel(data.right, 1);
          const source = ctx.createBufferSource();
          source.buffer = buffer;
          source.connect(gain);
          source.start(at);
          return;
        }
        case 'ready': {
          // 時計: 鳴っている位置を PTS にして送る
          clockTimer = setInterval(() => {
            if (p0 === null) return;
            const ts = ctx.getOutputTimestamp();
            if (!ts.contextTime || !ts.performanceTime) return;
            worker.postMessage({
              type: 'clock',
              pts: p0 + (ts.contextTime - t0) * SECOND,
              perf: ts.performanceTime,
            });
          }, clockIntervalMs);
          const tick = (t) => {
            worker.postMessage({ type: 'vsync', t });
            raf = requestAnimationFrame(tick);
          };
          raf = requestAnimationFrame(tick);
          return;
        }
        case 'result':
          stop(data.result);
          return;
        default:
      }
    };
    ctx
      .resume()
      .then(() =>
        worker.postMessage({ type: 'start', file, variant, canvas: offscreen, seconds }, [
          offscreen,
        ]),
      )
      .catch((error) => stop({ ok: false, kind: 'play', file, variant, error: String(error) }));
  });
