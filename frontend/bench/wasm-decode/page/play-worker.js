// WASM の FFmpeg でデコードし、WebGL2 で描いて実時間で再生する方式の測定 (#1516 の「測ること」)。
// 時計は、メインスレッドが鳴らしている音の位置 (getOutputTimestamp から出した PTS) で、画面の切り替わりの時刻もメインスレッドから受け取る。
// 測るもの: 段ごとの時間 (デコード、テクスチャへの上げ、描画)、切り替わりごとに出したフィールド (重複と飛び)、捨てたコマ、時計とのずれ。
// 読み込みは測定から外す (先に全部メモリに置く)。音は、鳴らす少し先までを順にデコードしてメインスレッドへ渡す。
import { demux } from './ts.js';

// AV_FRAME_FLAG_INTERLACED と AV_FRAME_FLAG_TOP_FIELD_FIRST
const FRAME_FLAG_INTERLACED = 8;
const FRAME_FLAG_TOP_FIELD_FIRST = 16;
// PTS の 1 秒 (90kHz)
const SECOND = 90_000;

// 決めるパラメーター (Issue の表)。ここでは測るための仮の値。
const params = {
  // デコードの先読み (PTS の時間)。表示時刻のこれだけ前までデコードしておく。
  lookahead: 0.2 * SECOND,
  // デコード済みの上限 (枚数)。テクスチャの輪の大きさもこれで決まる。
  maxDecoded: 8,
  // 音を、鳴っている位置のこれだけ先までデコードして渡す。
  audioAhead: 1.0 * SECOND,
  // 時計: 予測との差がこれを超えたら合わせ直し、以下ならこの割合だけ寄せる。
  clockResync: 0.02 * SECOND,
  clockBlend: 0.1,
  // 画面の切り替わりの間隔: 受け取る範囲 (ms) と、混ぜる割合。
  vsyncRange: [4, 50],
  vsyncBlend: 0.05,
  // 間合い: 切り替わりごとに進めた PTS と時計の差がこれ以内なら寄せ、超えたら合わせ直す。寄せる割合。
  ptsSnapRange: 0.025 * SECOND,
  ptsBlend: 0.02,
  // 間合いの補正を使うか (使わなければ、切り替わりの時刻の時計でそのまま選ぶ)。比べるために切り替えられる。
  pacing: true,
  // インタレ解除の閾値 (0〜1 の輝度)。
  weaveThreshold: 4 / 255,
  bobThreshold: 24 / 255,
  stillThreshold: 2 / 255,
  motionThreshold: 12 / 255,
};

const quantile = (values, q) => {
  if (values.length === 0) return null;
  const sorted = [...values].sort((a, b) => a - b);
  return sorted[Math.min(sorted.length - 1, Math.floor(q * sorted.length))];
};
const summarize = (values) => ({
  n: values.length,
  avg: values.length ? values.reduce((a, b) => a + b, 0) / values.length : null,
  p50: quantile(values, 0.5),
  p95: quantile(values, 0.95),
  p99: quantile(values, 0.99),
  max: values.length ? Math.max(...values) : null,
});

const vertexShader = `#version 300 es
void main() {
  // 画面全体を覆う 1 つの三角形
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

// フィールドごとに描く。描くフィールドの行はそのまま使い、抜けた行は、もう片方のフィールドの値 o が上下の行 a、b の間から
// はみ出した量で、織り (o) と bob ((a+b)/2) を混ぜる。はみ出しても、前のコマと比べて止まっている画素は o を使う。
const fragmentShader = `#version 300 es
precision highp float;
precision highp int;
uniform highp sampler2D uY;
uniform highp sampler2D uU;
uniform highp sampler2D uV;
uniform highp sampler2D uPrevY;
uniform int uField;      // 描くフィールド (0 = 上、1 = 下)
uniform int uInterlaced; // インタレースのコマか
uniform int uHasPrev;    // 前のコマがあるか
uniform vec2 uSource;    // 元の幅と高さ
uniform vec2 uOutput;    // 描く先の幅と高さ
uniform vec4 uThresholds; // 織り、bob、静止、動きの閾値
out vec4 fragColor;

float y(ivec2 p) { return texelFetch(uY, clamp(p, ivec2(0), ivec2(uSource) - 1), 0).r; }
float py(ivec2 p) { return texelFetch(uPrevY, clamp(p, ivec2(0), ivec2(uSource) - 1), 0).r; }

void main() {
  // 描く先は、行を下から数える。元の画像は上から数える。
  vec2 uv = vec2(gl_FragCoord.x / uOutput.x, 1.0 - gl_FragCoord.y / uOutput.y);
  ivec2 p = ivec2(uv * uSource);
  float luma;
  float keep = 1.0;
  if (uInterlaced == 0 || (p.y & 1) == uField) {
    luma = y(p);
  } else {
    float a = y(p - ivec2(0, 1));
    float b = y(p + ivec2(0, 1));
    float o = y(p);
    float over = max(0.0, max(o - max(a, b), min(a, b) - o));
    keep = 1.0 - smoothstep(uThresholds.x, uThresholds.y, over);
    if (over > uThresholds.x && uHasPrev == 1) {
      float motion = max(abs(o - py(p)), max(abs(a - py(p - ivec2(0, 1))), abs(b - py(p + ivec2(0, 1)))));
      keep = max(keep, 1.0 - smoothstep(uThresholds.z, uThresholds.w, motion));
    }
    luma = mix((a + b) * 0.5, o, keep);
  }
  ivec2 c = ivec2(uv * uSource * 0.5);
  float u = texelFetch(uU, c, 0).r;
  float v = texelFetch(uV, c, 0).r;
  // BT.709、限定レンジ
  float Y = (luma - 16.0 / 255.0) * (255.0 / 219.0);
  float Cb = (u - 128.0 / 255.0) * (255.0 / 224.0);
  float Cr = (v - 128.0 / 255.0) * (255.0 / 224.0);
  fragColor = vec4(Y + 1.5748 * Cr, Y - 0.1873 * Cb - 0.4681 * Cr, Y + 1.8556 * Cb, 1.0);
}`;

let state = null;

onmessage = async ({ data }) => {
  try {
    if (data.type === 'start') await start(data);
    else if (data.type === 'clock') onClock(data);
    else if (data.type === 'vsync') onVsync(data);
    else if (data.type === 'stop') finish('stopped');
  } catch (error) {
    postMessage({
      type: 'result',
      result: { ok: false, kind: 'play', error: String(error?.stack ?? error) },
    });
    state = null;
  }
};

async function start({ file, variant, canvas, seconds, pacing }) {
  if (pacing !== undefined) params.pacing = pacing;
  const { default: init } = await import(`./wasm/${variant}/dec.mjs`);
  const M = await init({ locateFile: (path) => `./wasm/${variant}/${path}` });
  const response = await fetch(`./media/${file}`);
  if (!response.ok) throw new Error(`failed to fetch ${file}: ${response.status}`);
  const { video, audio } = demux(await response.arrayBuffer());

  const gl = canvas.getContext('webgl2', { antialias: false, alpha: false, depth: false });
  if (gl === null) throw new Error('WebGL2 is not available in the worker');
  const program = compile(gl);
  const loc = (name) => gl.getUniformLocation(program, name);
  const uniforms = Object.fromEntries(
    [
      'uY',
      'uU',
      'uV',
      'uPrevY',
      'uField',
      'uInterlaced',
      'uHasPrev',
      'uSource',
      'uOutput',
      'uThresholds',
    ].map((n) => [n, loc(n)]),
  );
  const timer = gl.getExtension('EXT_disjoint_timer_query_webgl2');

  const vd = M._dec_open(0);
  const ad = M._dec_open(1);
  const outp = M._malloc(4);
  let cap = 1 << 20;
  let inp = M._malloc(cap);

  state = {
    M,
    gl,
    program,
    uniforms,
    timer,
    vd,
    ad,
    outp,
    video,
    audio,
    canvas,
    inp: () => inp,
    ensure: (n) => {
      if (n > cap) {
        M._free(inp);
        cap = n * 2;
        inp = M._malloc(cap);
      }
    },
    seconds,
    file,
    variant,
    // 映像
    nextVideoPes: 0,
    flushed: false,
    frames: [], // デコード済み (表示時刻の順): { pts, slot, interlaced, tff, fieldDur, shownFields }
    slots: [], // テクスチャの輪: { y, u, v, w, h, busy }
    lastPts: -Infinity,
    frameDur: null,
    // 音
    nextAudioPes: 0,
    audioSentUntil: -Infinity,
    // 時計 (PTS = offset + performance.now() * 90)
    clockOffset: null,
    // 画面の切り替わり
    vsyncInterval: null,
    lastVsync: null,
    // 出したフィールド: キーは「コマの PTS:フィールド」、値は出した回数
    shown: new Map(),
    shownOrder: [],
    current: null,
    // 測った値
    decodeMs: [],
    uploadMs: [],
    drawMs: [],
    gpuMs: [],
    audioDecodeMs: [],
    drift: [],
    dropped: 0,
    decodedFrames: 0,
    ticks: 0,
    vsyncIntervals: [],
    startedAt: null,
    pendingQueries: [],
    vsyncPts: null,
    ptsResyncs: 0,
    clockResyncs: 0,
    ptsDiff: [],
  };
  // 最初の音を渡す (メインスレッドは、届いた最初の音の PTS を鳴らし始めの位置にする)
  feedAudio(-Infinity);
  // 最初のコマを用意する
  decodeAhead(-Infinity);
  postMessage({ type: 'ready', firstVideoPts: state.frames[0]?.pts ?? null });
}

function compile(gl) {
  const shader = (type, source) => {
    const s = gl.createShader(type);
    gl.shaderSource(s, source);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(s));
    return s;
  };
  const program = gl.createProgram();
  gl.attachShader(program, shader(gl.VERTEX_SHADER, vertexShader));
  gl.attachShader(program, shader(gl.FRAGMENT_SHADER, fragmentShader));
  gl.linkProgram(program);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS))
    throw new Error(gl.getProgramInfoLog(program));
  gl.bindVertexArray(gl.createVertexArray());
  return program;
}

function texture(gl, w, h) {
  const t = gl.createTexture();
  gl.bindTexture(gl.TEXTURE_2D, t);
  gl.texStorage2D(gl.TEXTURE_2D, 1, gl.R8, w, h);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  return t;
}

// freeSlot は、使っていないテクスチャの組を返す。なければ作る。
function freeSlot(w, h) {
  const s = state;
  let slot = s.slots.find((x) => !x.busy && x.w === w && x.h === h);
  if (!slot) {
    const gl = s.gl;
    slot = {
      y: texture(gl, w, h),
      u: texture(gl, w >> 1, h >> 1),
      v: texture(gl, w >> 1, h >> 1),
      w,
      h,
      busy: false,
    };
    s.slots.push(slot);
  }
  slot.busy = true;
  return slot;
}

// upload は、WASM のメモリにあるデコードしたフレームの 3 面を、複製せずにテクスチャへ上げる。
function upload(slot) {
  const { M, gl, vd } = state;
  gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
  const planes = [
    [slot.y, slot.w, slot.h],
    [slot.u, slot.w >> 1, slot.h >> 1],
    [slot.v, slot.w >> 1, slot.h >> 1],
  ];
  planes.forEach(([t, w, h], i) => {
    gl.pixelStorei(gl.UNPACK_ROW_LENGTH, M._frame_linesize(vd, i));
    gl.bindTexture(gl.TEXTURE_2D, t);
    gl.texSubImage2D(
      gl.TEXTURE_2D,
      0,
      0,
      0,
      w,
      h,
      gl.RED,
      gl.UNSIGNED_BYTE,
      M.HEAPU8,
      M._frame_data(vd, i),
    );
  });
  gl.pixelStorei(gl.UNPACK_ROW_LENGTH, 0);
}

function drain() {
  const s = state;
  const { M, vd } = s;
  while (M._dec_receive(vd) === 0) {
    const pts = M._frame_pts(vd);
    const flags = M._frame_flags(vd);
    const w = M._frame_width(vd);
    const h = M._frame_height(vd);
    // コマの長さは、隣り合うコマの PTS の差のうち、いちばん小さいもの (コマが欠けた所の差を使わない)
    const delta = pts - s.lastPts;
    if (s.lastPts > -Infinity && delta > 0 && (s.frameDur === null || delta < s.frameDur))
      s.frameDur = delta;
    const interlaced = (flags & FRAME_FLAG_INTERLACED) !== 0;
    s.decodedFrames++;
    // 前より古い時刻のコマは捨てる
    if (pts <= s.lastPts) {
      s.dropped++;
      continue;
    }
    s.lastPts = pts;
    const t = performance.now();
    const slot = freeSlot(w, h);
    upload(slot);
    s.uploadMs.push(performance.now() - t);
    s.frames.push({ pts, slot, interlaced, tff: (flags & FRAME_FLAG_TOP_FIELD_FIRST) !== 0, w, h });
  }
}

// decodeAhead は、表示する位置 (PTS) のデコードの先読みの分まで、デコード済みの上限を超えない範囲でデコードする。
function decodeAhead(targetPts) {
  const s = state;
  const { M, vd, outp } = s;
  while (s.nextVideoPes < s.video.length || !s.flushed) {
    const newest = s.frames.at(-1);
    if (s.frames.length >= params.maxDecoded) break;
    if (newest && targetPts > -Infinity && newest.pts > targetPts + params.lookahead) break;
    if (newest && targetPts === -Infinity && s.frames.length >= 2) break;
    if (s.nextVideoPes >= s.video.length) {
      // 映像の終わり: parser に残った最後のコマと、並べ替えで残ったコマを出す
      s.flushed = true;
      M._dec_parse(vd, 0, 0, -1, outp);
      if (M.HEAP32[outp >> 2]) M._dec_send_parsed(vd);
      M._dec_send(vd, 0, 0, -1);
      drain();
      break;
    }
    const pes = s.video[s.nextVideoPes++];
    const t = performance.now();
    s.ensure(pes.data.length);
    M.HEAPU8.set(pes.data, s.inp());
    let off = 0;
    let uploadBefore = s.uploadMs.length;
    do {
      off += M._dec_parse(vd, s.inp() + off, pes.data.length - off, off === 0 ? pes.pts : -1, outp);
      if (M.HEAP32[outp >> 2]) {
        M._dec_send_parsed(vd);
        drain();
      }
    } while (off < pes.data.length);
    // デコードの時間からテクスチャへの上げの時間を引く
    const uploaded = s.uploadMs.slice(uploadBefore).reduce((a, b) => a + b, 0);
    s.decodeMs.push(performance.now() - t - uploaded);
  }
}

// feedAudio は、鳴っている位置の少し先までの音をデコードし、メインスレッドへ渡す。
function feedAudio(nowPts) {
  const s = state;
  const { M, ad } = s;
  while (
    s.nextAudioPes < s.audio.length &&
    (nowPts === -Infinity
      ? s.audioSentUntil === -Infinity
      : s.audioSentUntil < nowPts + params.audioAhead)
  ) {
    const pes = s.audio[s.nextAudioPes++];
    const d = pes.data;
    const t = performance.now();
    let k = 0;
    for (let o = 0; o + 7 <= d.length;) {
      if (d[o] !== 0xff || (d[o + 1] & 0xf0) !== 0xf0) {
        o++;
        continue;
      }
      const len = ((d[o + 3] & 3) << 11) | (d[o + 4] << 3) | (d[o + 5] >> 5);
      if (len < 7 || o + len > d.length) break;
      s.ensure(len);
      M.HEAPU8.set(d.subarray(o, o + len), s.inp());
      M._dec_send(ad, s.inp(), len, -1);
      while (M._dec_receive(ad) === 0) {
        const n = M._frame_nb_samples(ad);
        const left = new Float32Array(M.HEAPU8.buffer, M._frame_data(ad, 0), n).slice();
        const rightPtr = M._frame_data(ad, 1);
        const right = rightPtr
          ? new Float32Array(M.HEAPU8.buffer, rightPtr, n).slice()
          : left.slice();
        // 1 フレームは 1024 サンプル、48kHz
        const pts = pes.pts + Math.round((k * 1024 * SECOND) / 48_000);
        k++;
        s.audioSentUntil = pts + Math.round((n * SECOND) / 48_000);
        postMessage({ type: 'audio', pts, left, right }, [left.buffer, right.buffer]);
      }
      o += len;
    }
    s.audioDecodeMs.push(performance.now() - t);
  }
}

// onClock は、メインスレッドが送る「いま耳に届いている PTS」と performance の時刻の組で、時計を合わせる。
function onClock({ pts, perf }) {
  const s = state;
  if (!s) return;
  const offset = pts - perf * 90;
  if (s.clockOffset === null || Math.abs(offset - s.clockOffset) > params.clockResync) {
    if (s.clockOffset !== null) s.clockResyncs++;
    s.clockOffset = offset;
  } else s.clockOffset += (offset - s.clockOffset) * params.clockBlend;
}

// onVsync は、画面が切り替わった時刻 (requestAnimationFrame の時刻) を受け取り、次の切り替わりに出すフィールドを描く。
function onVsync({ t }) {
  const s = state;
  if (!s) return;
  let steps = 1;
  if (s.lastVsync !== null) {
    const dt = t - s.lastVsync;
    // 何回分か抜けた回は、回数で割って 1 回分に戻してから覚える
    steps = s.vsyncInterval === null ? 1 : Math.max(1, Math.round(dt / s.vsyncInterval));
    const one = dt / steps;
    if (one >= params.vsyncRange[0] && one <= params.vsyncRange[1]) {
      s.vsyncInterval =
        s.vsyncInterval === null
          ? one
          : s.vsyncInterval + (one - s.vsyncInterval) * params.vsyncBlend;
      s.vsyncIntervals.push(one);
    }
  }
  s.lastVsync = t;
  if (s.clockOffset === null || s.vsyncInterval === null) return;
  // 次の切り替わりの時刻の時計
  const clock = s.clockOffset + (t + s.vsyncInterval) * 90;
  let target = clock;
  if (params.pacing) {
    // 切り替わりごとの PTS は、前の切り替わりの PTS に間隔の分を足して進め、時計との差が寄せる範囲の中なら少しずつ寄せる。
    // 出す時刻の揺れで、同じフィールドが 2 回出て次が飛ぶのを避けるため。範囲を超えたら (飛んだ、貯め直した) 時計に合わせ直す。
    if (s.vsyncPts === undefined || s.vsyncPts === null) s.vsyncPts = clock;
    else {
      const stepped = s.vsyncPts + steps * s.vsyncInterval * 90;
      const diff = clock - stepped;
      if (Math.abs(diff) <= params.ptsSnapRange) s.vsyncPts = stepped + diff * params.ptsBlend;
      else {
        s.vsyncPts = clock;
        s.ptsResyncs++;
      }
      s.ptsDiff.push(diff / 90);
    }
    target = s.vsyncPts;
  }
  if (s.startedAt === null) s.startedAt = target;
  feedAudio(target);
  decodeAhead(target);
  draw(target);
  if (
    (target - s.startedAt) / SECOND >= s.seconds ||
    (s.flushed && s.frames.length <= 1 && target >= (s.frames[0]?.pts ?? 0) + (s.frameDur ?? 3003))
  )
    finish('done');
}

function draw(target) {
  const s = state;
  const { gl, uniforms } = s;
  const fieldDur = (s.frameDur ?? 3003) / 2;
  // 表示時刻を過ぎたコマは、次のコマが出せるなら捨てる (2 枚目のフィールドの番も過ぎたもの)
  while (s.frames.length >= 2 && s.frames[1].pts <= target) {
    const old = s.frames.shift();
    if (!s.shown.has(`${old.pts}:0`) && !s.shown.has(`${old.pts}:1`)) s.dropped++;
    if (s.prevSlot && s.prevSlot !== old.slot) s.prevSlot.busy = false;
    s.prevSlot = old.slot;
  }
  const frame = s.frames[0];
  if (!frame || frame.pts > target) return;
  const second = frame.interlaced && target >= frame.pts + fieldDur ? 1 : 0;
  // 出すフィールドの、画像の上下 (TFF なら 1 枚目が上)
  const parity = frame.interlaced ? ((second === 0) === frame.tff ? 0 : 1) : 0;
  const key = `${frame.pts}:${second}`;
  s.shown.set(key, (s.shown.get(key) ?? 0) + 1);
  s.shownOrder.push(key);
  s.drift.push((target - (frame.pts + second * fieldDur)) / 90);
  s.ticks++;

  const t = performance.now();
  let query = null;
  if (s.timer) {
    query = gl.createQuery();
    gl.beginQuery(s.timer.TIME_ELAPSED_EXT, query);
  }
  gl.viewport(0, 0, s.canvas.width, s.canvas.height);
  gl.useProgram(s.program);
  const bind = (unit, tex, name) => {
    gl.activeTexture(gl.TEXTURE0 + unit);
    gl.bindTexture(gl.TEXTURE_2D, tex);
    gl.uniform1i(uniforms[name], unit);
  };
  bind(0, frame.slot.y, 'uY');
  bind(1, frame.slot.u, 'uU');
  bind(2, frame.slot.v, 'uV');
  // 前のコマは、大きさが同じときだけ比べる (選局した直後と、大きさが変わった直後は、コマの中だけで決める)
  const hasPrev = Boolean(s.prevSlot && s.prevSlot.w === frame.w && s.prevSlot.h === frame.h);
  bind(3, hasPrev ? s.prevSlot.y : frame.slot.y, 'uPrevY');
  gl.uniform1i(uniforms.uField, parity);
  gl.uniform1i(uniforms.uInterlaced, frame.interlaced ? 1 : 0);
  gl.uniform1i(uniforms.uHasPrev, hasPrev ? 1 : 0);
  gl.uniform2f(uniforms.uSource, frame.w, frame.h);
  gl.uniform2f(uniforms.uOutput, s.canvas.width, s.canvas.height);
  gl.uniform4f(
    uniforms.uThresholds,
    params.weaveThreshold,
    params.bobThreshold,
    params.stillThreshold,
    params.motionThreshold,
  );
  gl.drawArrays(gl.TRIANGLES, 0, 3);
  if (query) {
    gl.endQuery(s.timer.TIME_ELAPSED_EXT);
    s.pendingQueries.push(query);
  }
  s.drawMs.push(performance.now() - t);
  // 前に頼んだ GPU の時間を、出ていれば読む
  while (s.pendingQueries.length > 0) {
    const q = s.pendingQueries[0];
    if (!gl.getQueryParameter(q, gl.QUERY_RESULT_AVAILABLE)) break;
    if (!gl.getParameter(s.timer.GPU_DISJOINT_EXT))
      s.gpuMs.push(gl.getQueryParameter(q, gl.QUERY_RESULT) / 1e6);
    gl.deleteQuery(q);
    s.pendingQueries.shift();
  }
}

function finish(reason) {
  const s = state;
  if (!s) return;
  state = null;
  // 出したフィールドの数え上げ。最初と最後のコマは、途中から始まり途中で終わるので数えない。
  const keys = [...new Set(s.shownOrder)];
  const holds = keys.slice(1, -1).map((k) => s.shown.get(k));
  const holdHistogram = {};
  for (const h of holds) holdHistogram[h] = (holdHistogram[h] ?? 0) + 1;
  // 飛んだフィールド: 出したフィールドの並びで、間のフィールドが出ていないもの
  let skipped = 0;
  const seq = keys.map((k) => k.split(':').map(Number));
  for (let i = 1; i < seq.length; i++) {
    const [pa, fa] = seq[i - 1];
    const [pb, fb] = seq[i];
    const stepFields = Math.round((pb - pa) / ((s.frameDur ?? 3003) / 2) + fb - fa);
    if (stepFields > 1) skipped += stepFields - 1;
  }
  const intervals = summarize(s.vsyncIntervals);
  postMessage({
    type: 'result',
    result: {
      ok: true,
      kind: 'play',
      file: s.file,
      variant: s.variant,
      pacing: params.pacing,
      ptsResyncs: s.ptsResyncs,
      clockResyncs: s.clockResyncs,
      ptsDiffMs: summarize(s.ptsDiff.map(Math.abs)),
      reason,
      seconds: s.startedAt === null ? 0 : s.seconds,
      canvas: [s.canvas.width, s.canvas.height],
      frameDur: s.frameDur,
      decodedFrames: s.decodedFrames,
      droppedFrames: s.dropped,
      ticks: s.ticks,
      vsyncMs: intervals,
      fieldsShown: keys.length,
      fieldHolds: holdHistogram,
      fieldsSkipped: skipped,
      driftMs: summarize(s.drift),
      decodeMs: summarize(s.decodeMs),
      uploadMs: summarize(s.uploadMs),
      drawMs: summarize(s.drawMs),
      gpuMs: s.timer ? summarize(s.gpuMs) : null,
      audioDecodeMs: summarize(s.audioDecodeMs),
    },
  });
}
