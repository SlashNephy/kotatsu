/// <reference lib="webworker" />
// ブラウザ内でデコードする経路 (#1516、設計書 7.2) の worker。
// TS を読んで PES に分け (tsDemux)、映像と音声を WASM の FFmpeg でデコードし、映像は OffscreenCanvas に WebGL2 で描き
// (pictureRenderer)、音声はメインスレッドへ渡す。時計は、メインスレッドが鳴らしている音 (decodeClock) で、
// コマを出す間合いは、メインスレッドから届く画面の切り替わりの時刻で決める (fieldPacer)。
// 間に合っているかは decodeJudge で判定し、少し遅れたら B フレームを省き、間に合わなければメインスレッドへ通知する。
import createDecoderModule, { type DecoderModule } from '@kotatsu/ffmpeg-decoder';

import { AdtsSplitter } from './adtsFrames';
import { chooseAudioPid, type DualMonoChannel, routeDualMono, startsWithPce } from './decodeAudio';
import { DecodeClock } from './decodeClock';
import { DecodeJudge, type Verdict } from './decodeJudge';
import type {
  DecodeAudioStream,
  DecodeStats,
  FromDecodeWorker,
  ToDecodeWorker,
} from './decodeProtocol';
import { FieldPacer } from './fieldPacer';
import { PictureQueue } from './pictureQueue';
import { type ColorMatrix, type PictureSlot, PictureRenderer } from './pictureRenderer';
import { PtsTimeline, ptsTicksPerSecond } from './ptsTimeline';
import { decodeStreamEndpoint } from './route';
import { type TsPes, TsDemuxer } from './tsDemux';

declare const self: DedicatedWorkerGlobalScope;

// params は、#1516 の「決めるパラメーター」のうち、worker が使うもの (PTS の単位)。
const params = {
  // lookahead は、映す PTS のこれだけ前までデコードしておく量。重いコマのデコード時間より長くする。
  lookahead: 0.2 * ptsTicksPerSecond,
  // maxDecoded は、デコードして待たせるコマの上限。並べ替え (B フレーム) と先読みに要る枚数より多くする。
  maxDecoded: 8,
  // audioAhead は、鳴っている位置のこれだけ先まで、音をデコードして渡す量。
  audioAhead: 1 * ptsTicksPerSecond,
  // maxCompressed は、デコードしていない映像の PES を持つ長さの上限。止めている間にバッファし続けない。
  maxCompressed: 10 * ptsTicksPerSecond,
  // audioKeep は、鳴っている位置より前の音声のフレームを残す長さ。選び直したら、鳴っている位置から渡し直すのに使う (#2165)。
  audioKeep: 0.5 * ptsTicksPerSecond,
  // readAhead は、映している位置より先に読んでおく映像の長さの上限。録画の配信は等速より速く届くので、
  // これを超えたら、映して減るまで読むのを待つ (#2166)。maxCompressed より短くし、映す前のデータを破棄しない。
  readAhead: 5 * ptsTicksPerSecond,
};

// AVFrame の flags (libavutil/frame.h)。
const frameFlagInterlaced = 1 << 3;
const frameFlagTopFieldFirst = 1 << 4;
// AV_SAMPLE_FMT_FLTP (libavutil/samplefmt.h)。AAC のデコーダは、この形で出す。
const sampleFormatFloatPlanar = 8;
// AVColorSpace のうち、BT.601 のもの (BT470BG、SMPTE170M)。
const bt601ColorSpaces = new Set([5, 6]);
// 映像の PES に sequence_header_code (00 00 01 B3) があれば、そこから GOP が始まる (I フレームから映せる)。
const sequenceHeader = [0x00, 0x00, 0x01, 0xb3];

type CompressedVideo = { pts: number | null; data: Uint8Array; keyframe: boolean };
// CompressedAudio は、デコードしていない ADTS のフレーム。channels は channel_configuration (0 はデュアルモノ)。
type CompressedAudio = { pts: number; duration: number; channels: number; data: Uint8Array };

// AudioEs は、番組の音声の ES 1 つの、デコードしていないフレーム。鳴らさない ES も、選び直したときのために少し残す (#2165)。
type AudioEs = {
  componentTag: number | null;
  adts: AdtsSplitter;
  frames: CompressedAudio[];
  // dualMono は、デュアルモノ (channel_configuration が 0) のフレームが届いたか。
  dualMono: boolean;
};

type Loading = {
  load: number;
  abort: AbortController;
  demuxer: TsDemuxer;
  timeline: PtsTimeline;
  video: CompressedVideo[];
  // audios は、番組の音声の ES (PMT の順)。
  audios: Map<number, AudioEs>;
  origin: number | null;
  // audioSentUntil は、渡した音の終わりの PTS。
  audioSentUntil: number | null;
  // audioCursor は、デコーダに渡したフレームの終わりの PTS。これより後のフレームから渡す。
  audioCursor: number | null;
  // cutting は、選び直した音をどこから渡すか、メインスレッドの返事 (audioFrom) を待っている切り替えの番号。
  // 待っていなければ null。待つ間は音を渡さない。続けて選び直したら、前の切り替えの返事は使わない。
  cutting: number | null;
  // pceSeen は、デュアルモノの ES で、チャンネルの構成 (PCE) をデコーダに渡したか。渡すまでは、デコードできないフレームを渡さない。
  // デコーダは、dec_flush でやり直しても、読んだ構成を保持している。デュアルモノでないフレームを渡したら、その構成に替わるので戻す。
  pceSeen: boolean;
  presented: boolean;
  shape: string | null;
  bytesRead: number;
  // waitKeyframe は、I フレームから始まる PES まで映像を破棄するか (読み始め、見えるようになった直後)。
  waitKeyframe: boolean;
  // audioPid は、鳴らす音声の ES の PID。選んだ ES (audioSelection) が番組になければ、PMT の最初の音声。
  audioPid: number | null;
  // audioStart は、時計が動く前に渡し始めた音の PTS。時計が動くまでは、ここから先読みの分だけ渡す。
  audioStart: number | null;
  // lastVideoPts は、最後に届いた映像の PES の PTS。時刻を持たない文字スーパーの時刻に使う。
  lastVideoPts: number | null;
  // nextPts は、PTS のないコマに当てる時刻。
  nextPts: number | null;
  // shownUntil は、最後に描いたコマの終わりの PTS。映すコマが時計からどれだけ遅れたかを出すのに使う。
  shownUntil: number | null;
  // droppedSeen は、判定へ渡し終えた、映さずに破棄したコマの数。
  droppedSeen: number;
  // drained は、先読みが上限を超えて読むのを待っている間の、待ちを解く関数。待っていなければ null。
  drained: (() => void) | null;
  // eof は、配信を読み終えたか。読み終えても、読んだ分を映し終えるまでは ended を送らない。
  eof: boolean;
  endedSent: boolean;
};

type Engine = {
  module: DecoderModule;
  videoDecoder: number;
  audioDecoder: number;
  renderer: PictureRenderer;
  queue: PictureQueue<PictureSlot>;
  // input は、WASM へデータを渡す領域。
  input: { pointer: number; size: number };
  // flag は、dec_parse が「切り出せたか」を書く 4 バイト。
  flag: number;
  matrix: ColorMatrix;
};

let engine: Engine | null = null;
// mainOrigin は、メインスレッドの performance.timeOrigin。mainNow で、メインスレッドの performance の時刻にそろえる。
let mainOrigin = performance.timeOrigin;

// mainNow は、今の時刻を、メインスレッドの performance の時刻で返す。時計 (decodeClock) はメインスレッドの時刻で合わせるため。
function mainNow(): number {
  return performance.now() + performance.timeOrigin - mainOrigin;
}
let loading: Loading | null = null;
// cuts は、音声を選び直した回数。メインスレッドとの切り替えのやり取りに番号として付ける。
let cuts = 0;
// audioSelection は、利用者が選んだ音声 (#2165)。読み込み直しても保ち、番組にその ES がなければ最初の ES を鳴らす。
const audioSelection: { pid: number | null; dualMono: DualMonoChannel } = {
  pid: null,
  dualMono: 'main',
};
const clock = new DecodeClock();
const pacer = new FieldPacer();
let visible = true;
let presented = 0;
const decodeTimes: number[] = [];
const judge = new DecodeJudge();
let statsTimer: ReturnType<typeof setInterval> | null = null;

function post(message: FromDecodeWorker, transfer: Transferable[] = []): void {
  self.postMessage(message, transfer);
}

self.onmessage = (event: MessageEvent<ToDecodeWorker>) => {
  // 専用の Worker に送れるのは、作った画面のスレッドだけで、そのメッセージの origin は空になる (Chromium と Firefox)。
  // 空にしないブラウザでも通るよう、同じ origin も受け、ほかの origin のものは読まない。
  if (event.origin !== '' && event.origin !== self.location.origin) {
    return;
  }
  const message = event.data;
  switch (message.type) {
    case 'start':
      mainOrigin = message.timeOrigin;
      void start(message.canvas, message.wasmUrl);
      break;
    case 'load':
      load(message.load, message.url, message.serviceId);
      break;
    case 'stop':
      stop();
      break;
    case 'clock':
      if (loading?.load === message.load) {
        const started = clock.at(mainNow()) === null;
        clock.observe(message.pts, message.perf);
        // 時計が動き出したときは、映す時刻が飛ぶ。追いつくまでの間は、間に合っているかを数えない。
        // 合わせ直したときは数える。間に合わない端末では、音が途切れて合わせ直しが続き、数えないままになるため。
        if (started) {
          judge.resume(performance.now());
        }
      }
      break;
    case 'vsync':
      onVsync(message.time);
      break;
    case 'visible':
      setVisible(message.visible);
      break;
    case 'selectAudio':
      selectAudio(message.pid, message.dualMono);
      break;
    case 'audioFrom':
      if (loading?.load === message.load && loading.cutting === message.cut) {
        resumeAudio(loading, message.pts);
      }
      break;
  }
};

async function start(canvas: OffscreenCanvas, wasmUrl: string): Promise<void> {
  try {
    const module = await createDecoderModule({ locateFile: () => wasmUrl });
    const videoDecoder = module._dec_open(0);
    const audioDecoder = module._dec_open(1);
    if (videoDecoder === 0 || audioDecoder === 0) {
      throw new Error('failed to open the decoders');
    }
    const renderer = new PictureRenderer(canvas);
    canvas.addEventListener('webglcontextlost', () => {
      post({ type: 'error', load: loading?.load ?? null, message: 'the WebGL context was lost' });
    });
    const flag = module._buf_alloc(4);
    engine = {
      module,
      videoDecoder,
      audioDecoder,
      renderer,
      queue: new PictureQueue((slot) => renderer.release(slot)),
      input: { pointer: 0, size: 0 },
      flag,
      matrix: 'bt709',
    };
    post({ type: 'ready' });
  } catch (error) {
    console.error('failed to start the decoder', error);
    post({ type: 'error', load: null, message: errorMessage(error) });
  }
}

function load(load: number, url: string, serviceId: number | null): void {
  stop();
  const timeline = new PtsTimeline();
  const abort = new AbortController();
  const current: Loading = {
    load,
    abort,
    timeline,
    video: [],
    audios: new Map(),
    origin: null,
    audioSentUntil: null,
    audioCursor: null,
    cutting: null,
    pceSeen: false,
    presented: false,
    shape: null,
    bytesRead: 0,
    waitKeyframe: true,
    nextPts: null,
    shownUntil: null,
    droppedSeen: 0,
    drained: null,
    eof: false,
    endedSent: false,
    audioPid: null,
    audioStart: null,
    lastVideoPts: null,
    demuxer: new TsDemuxer({
      serviceId,
      onProgram: (program) => {
        // 番組の音声の ES を読み直す。番組から消えた ES のフレームは破棄する。
        const audios = new Map<number, AudioEs>();
        for (const stream of program.audios) {
          audios.set(
            stream.pid,
            current.audios.get(stream.pid) ?? {
              componentTag: stream.componentTag,
              adts: new AdtsSplitter(),
              frames: [],
              dualMono: false,
            },
          );
        }
        current.audios = audios;
        // 鳴らすのは 1 つの ES だけ。複数の ES を 1 つのデコーダに混ぜると、音が壊れる。
        const pid = chooseAudioPid([...audios.keys()], audioSelection.pid);
        if (current.audioPid !== pid) {
          current.audioPid = pid;
          restartAudio(current);
        }
        postAudioTracks(current);
      },
      onPes: (pes) => onPes(current, pes),
    }),
  };
  loading = current;
  judge.start(performance.now());
  statsTimer = setInterval(() => {
    sendStats(current);
    judgeTick(current);
  }, 1_000);
  void read(current, url);
}

function stop(): void {
  if (statsTimer !== null) {
    clearInterval(statsTimer);
    statsTimer = null;
  }
  if (loading !== null) {
    loading.abort.abort();
    loading = null;
  }
  clock.reset();
  pacer.reset();
  presented = 0;
  decodeTimes.length = 0;
  if (engine !== null) {
    if (judge.skippingB) {
      engine.module._dec_skip_b(engine.videoDecoder, 0);
    }
    engine.queue.clear();
    // 映さずに破棄した数は、読み込みごとに数え直す (getVideoPlaybackQuality と同じ)。
    engine.queue.dropped = 0;
    engine.module._dec_flush(engine.videoDecoder);
    engine.module._dec_flush(engine.audioDecoder);
  }
}

async function read(current: Loading, url: string): Promise<void> {
  try {
    // 受け取った URL をそのまま読まずに、ライブ視聴か録画の TS のパスへ組み直し、ほかのホストやパスを読まないようにする。
    const endpoint = decodeStreamEndpoint(url);
    if (endpoint === null) {
      throw new Error('the stream is neither a live stream nor a recording');
    }
    const response = await fetch(endpoint, {
      signal: current.abort.signal,
      cache: 'no-store',
      credentials: 'same-origin',
    });
    if (!response.ok || response.body === null) {
      throw new Error(`failed to read the stream: ${response.status}`);
    }
    const reader = response.body.getReader();
    current.abort.signal.addEventListener('abort', () => wakeReader(current));
    for (;;) {
      // 先に読んだ分が上限を超えたら、映して減るまで待つ。ライブは等速で届くので、待つことはない。
      while (readAheadOf(current) > params.readAhead && !current.abort.signal.aborted) {
        await new Promise<void>((resolve) => {
          current.drained = resolve;
        });
      }
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      current.bytesRead += value.length;
      current.demuxer.push(value);
    }
    current.demuxer.flush();
    // 読み終えたら、読んだ分を映し終えたときに ended を送る (onVsync)。見えていない間は映さないので、すぐ送る。
    current.eof = true;
    if (!visible && loading === current) {
      current.endedSent = true;
      post({ type: 'ended', load: current.load });
    }
  } catch (error) {
    if (!current.abort.signal.aborted) {
      post({ type: 'error', load: current.load, message: errorMessage(error) });
    }
  }
}

// readAheadOf は、映している位置より先に読んである映像の長さ (PTS の単位)。時計が動く前は、最初の映像から数える。
function readAheadOf(current: Loading): number {
  const newest = current.video.at(-1)?.pts ?? null;
  const now = clock.at(mainNow()) ?? current.video[0]?.pts ?? null;

  return newest === null || now === null ? 0 : newest - now;
}

// wakeReader は、先読みが上限を超えて待っている読み込みに、先読みの長さを確認し直させる。
function wakeReader(current: Loading): void {
  const drained = current.drained;
  current.drained = null;
  drained?.();
}

function onPes(current: Loading, pes: TsPes): void {
  const pts = pes.pts === null ? null : current.timeline.extend(pes.pts);
  switch (pes.kind) {
    case 'video': {
      // sequence_header は PES の先頭に置かれるので、先頭の方だけを探す。
      const keyframe = includes(pes.data.subarray(0, 512), sequenceHeader);
      if (current.waitKeyframe) {
        // I フレームが来るまで映像を渡さない。P や B を先に渡すと、灰色の映像が出る。
        if (!keyframe) {
          return;
        }
        current.waitKeyframe = false;
      }
      if (pts !== null) {
        current.lastVideoPts = pts;
      }
      judge.onVideo(performance.now());
      current.video.push({ pts, data: pes.data, keyframe });
      trimVideo(current);
      // まだ時計がなければ、最初のコマを出すために、ここでデコードを進める。
      if (clock.at(mainNow()) === null) {
        decodeVideo(current, null);
      }
      break;
    }
    case 'audio': {
      // 鳴らさない ES も、選び直したときに鳴っている位置から渡せるよう、フレームに分けて少し残す (#2165)。
      const es = current.audios.get(pes.pid);
      if (es === undefined) {
        return;
      }
      const playing = pes.pid === current.audioPid;
      for (const frame of es.adts.push(pes.data, pts)) {
        // 鳴らす ES の ADTS のフレームを切り出せたときだけ、音が届いたとみなす (ADTS でない内容なら、音が来ていない)。
        if (playing) {
          judge.onAudio();
        }
        es.frames.push({
          pts: frame.pts,
          duration: frame.duration,
          channels: frame.channels,
          data: frame.data,
        });
        // 番組や CM が替わると、同じ ES がデュアルモノになったり、ならなくなったりする。
        const dualMono = frame.channels === 0;
        if (dualMono !== es.dualMono) {
          es.dualMono = dualMono;
          postAudioTracks(current);
        }
      }
      trimAudio(current);
      if (playing) {
        feedAudio(current, clock.at(mainNow()));
      }
      break;
    }
    case 'captions':
    case 'superimpose': {
      // 文字スーパーは時刻を持たないので、届いたときの映像の時刻で出す。
      const time = pts ?? current.lastVideoPts;
      if (time !== null) {
        const data = pes.data.slice().buffer;
        post({ type: 'caption', load: current.load, kind: pes.kind, pts: time, data }, [data]);
      }
      break;
    }
  }
}

// trimAudio は、音声のフレームを、要らなくなった古い方から破棄する。時計が動いていれば、鳴っている位置より audioKeep 前まで残す。
// 時計が動く前 (鳴らし始める前) も、最新から maxCompressed を超えた分は破棄する。音ありの自動再生を拒否されている間に、
// バッファし続けないため。
function trimAudio(current: Loading): void {
  const now = clock.at(mainNow());
  for (const es of current.audios.values()) {
    const newest = es.frames.at(-1)?.pts ?? null;
    while (es.frames.length > 0) {
      const oldest = es.frames[0]!;
      const played = now !== null && oldest.pts + oldest.duration < now - params.audioKeep;
      const overflow = newest !== null && newest - oldest.pts > params.maxCompressed;
      if (!played && !overflow) {
        break;
      }
      es.frames.shift();
    }
  }
}

// selectAudio は、鳴らす音声を選び直す (#2165)。鳴らす ES かデュアルモノの振り分けが変わったら、鳴っている位置から渡し直す。
function selectAudio(pid: number | undefined, dualMono: DualMonoChannel | undefined): void {
  const before = { ...audioSelection };
  if (pid !== undefined) {
    audioSelection.pid = pid;
  }
  if (dualMono !== undefined) {
    audioSelection.dualMono = dualMono;
  }
  const current = loading;
  if (current === null) {
    return;
  }
  const next = chooseAudioPid([...current.audios.keys()], audioSelection.pid);
  const remixed =
    audioSelection.dualMono !== before.dualMono &&
    next !== null &&
    current.audios.get(next)?.dualMono === true;
  if (next !== current.audioPid || remixed) {
    current.audioPid = next;
    restartAudio(current);
  }
  postAudioTracks(current);
}

// restartAudio は、鳴らす音声を選び直したときに、デコーダをやり直し、メインスレッドに予約した音を破棄させる。
// どこから渡し直すかは、メインスレッドの返事 (audioFrom) で決める。
function restartAudio(current: Loading): void {
  if (engine !== null) {
    engine.module._dec_flush(engine.audioDecoder);
  }
  cuts += 1;
  current.cutting = cuts;
  post({ type: 'audioCut', load: current.load, cut: cuts });
}

// resumeAudio は、メインスレッドが予約した音を破棄した位置 pts から、選び直した音声を渡し直す。
// pts が null なら、まだ鳴らし始めていないので、渡し始めた位置から渡し直す。
function resumeAudio(current: Loading, pts: number | null): void {
  current.cutting = null;
  current.audioCursor = pts;
  current.audioSentUntil = pts;
  if (pts === null) {
    // 鳴らし始める前なら、選び直した ES のデコードできる最初のフレームから、渡し始めの位置を決め直す。
    current.audioStart = null;
  }
  feedAudio(current, clock.at(mainNow()));
}

// postAudioTracks は、番組の音声の ES と、鳴らしているものを、メインスレッドへ通知する。
function postAudioTracks(current: Loading): void {
  const streams: DecodeAudioStream[] = [...current.audios].map(([pid, es]) => ({
    pid,
    componentTag: es.componentTag,
    dualMono: es.dualMono,
  }));
  post({
    type: 'audioTracks',
    load: current.load,
    streams,
    current: current.audioPid,
    dualMono: audioSelection.dualMono,
  });
}

// trimVideo は、デコードしていない映像が上限を超えたら、古い GOP から破棄する。見えていない間は、最新の GOP だけを残す。
function trimVideo(current: Loading): void {
  if (!visible) {
    // 時刻を見ずに、最新の I フレームより前を破棄する (PTS のない PES があってもバッファし続けない)。
    const keyframe = current.video.findLastIndex((item) => item.keyframe);
    if (keyframe > 0) {
      current.video.splice(0, keyframe);
    }

    return;
  }
  const newest = current.video.at(-1)?.pts ?? null;
  const limit = params.maxCompressed;
  while (current.video.length > 1) {
    const first = current.video[0]!;
    const nextKeyframe = current.video.findIndex((item, i) => i > 0 && item.keyframe);
    if (nextKeyframe < 0) {
      return;
    }
    const tooLong = newest !== null && first.pts !== null && newest - first.pts > limit;
    if (!tooLong) {
      return;
    }
    current.video.splice(0, nextKeyframe);
  }
}

// ensureInput は、WASM へデータを渡す領域を size バイト以上にする。
function ensureInput(size: number): number {
  const target = engine!;
  if (target.input.size < size) {
    if (target.input.pointer !== 0) {
      target.module._buf_free(target.input.pointer);
    }
    const next = Math.max(size, target.input.size * 2, 1 << 16);
    target.input.pointer = target.module._buf_alloc(next);
    target.input.size = next;
  }

  return target.input.pointer;
}

// decodeVideo は、映す PTS target の先読みの分まで、待たせる上限を超えない範囲で映像をデコードする。
// target が null (まだ時計がない) なら、最初のコマを出すまでデコードする。
function decodeVideo(current: Loading, target: number | null): void {
  if (engine === null || !visible) {
    return;
  }
  const { module, videoDecoder, queue, flag } = engine;
  while (current.video.length > 0) {
    if (queue.length >= params.maxDecoded) {
      return;
    }
    const newest = queue.newest;
    if (
      target === null ? queue.length >= 1 : newest !== null && newest > target + params.lookahead
    ) {
      return;
    }
    const item = current.video.shift()!;
    const started = performance.now();
    const pointer = ensureInput(item.data.length);
    module.HEAPU8.set(item.data, pointer);
    let offset = 0;
    do {
      const used = module._dec_parse(
        videoDecoder,
        pointer + offset,
        item.data.length - offset,
        offset === 0 && item.pts !== null ? item.pts : Number.NaN,
        flag,
      );
      if (used < 0) {
        break;
      }
      offset += used;
      if (module.HEAP32[flag >> 2] !== 0) {
        module._dec_send_parsed(videoDecoder);
        receivePictures(current);
      }
    } while (offset < item.data.length);
    const elapsed = performance.now() - started;
    judge.onDecode(elapsed);
    decodeTimes.push(elapsed);
    if (decodeTimes.length > 120) {
      decodeTimes.shift();
    }
  }
}

// receivePictures は、デコードしたコマを受け取り、テクスチャに上げて待ち行列に足す。
function receivePictures(current: Loading): void {
  const target = engine!;
  const { module, videoDecoder, renderer, queue } = target;
  while (module._dec_receive(videoDecoder) === 0) {
    const width = module._frame_width(videoDecoder);
    const height = module._frame_height(videoDecoder);
    const flags = module._frame_flags(videoDecoder);
    const interlaced = (flags & frameFlagInterlaced) !== 0;
    let pts: number;
    if (module._frame_has_pts(videoDecoder) !== 0) {
      pts = module._frame_pts(videoDecoder);
    } else if (current.nextPts !== null) {
      pts = current.nextPts;
    } else {
      continue;
    }
    current.nextPts = pts + (queue.frameDuration ?? 3003);
    const sarNum = module._frame_sar_num(videoDecoder);
    const sarDen = module._frame_sar_den(videoDecoder);
    const displayWidth = sarNum > 0 && sarDen > 0 ? Math.round((width * sarNum) / sarDen) : width;
    const colorSpace = module._frame_colorspace(videoDecoder);
    target.matrix =
      bt601ColorSpaces.has(colorSpace) || (colorSpace === 2 && height < 720) ? 'bt601' : 'bt709';
    // 大きさが変わったときだけ通知する (走査の方式は、プルダウンの素材ではコマごとに変わりうるので含めない)。
    const shape = `${displayWidth}x${height}`;
    if (current.shape !== shape) {
      current.shape = shape;
      renderer.resize(displayWidth, height);
      post({ type: 'picture', load: current.load, width: displayWidth, height, interlaced });
    }
    const slot = renderer.acquire(width, height);
    renderer.upload(slot, module.HEAPU8, [
      {
        pointer: module._frame_data(videoDecoder, 0),
        stride: module._frame_linesize(videoDecoder, 0),
      },
      {
        pointer: module._frame_data(videoDecoder, 1),
        stride: module._frame_linesize(videoDecoder, 1),
      },
      {
        pointer: module._frame_data(videoDecoder, 2),
        stride: module._frame_linesize(videoDecoder, 2),
      },
    ]);
    const added = queue.push({
      pts,
      interlaced,
      topFieldFirst: (flags & frameFlagTopFieldFirst) !== 0,
      slot,
    });
    if (added) {
      judge.onPicture();
    }
    if (added && current.origin === null) {
      current.origin = pts;
      post({ type: 'origin', load: current.load, pts });
    }
    if (added && !current.presented && clock.at(mainNow()) === null) {
      // 時計が動く前 (音を貯めている間) は、最初のコマを出して待つ。
      present(current, pts);
    }
  }
}

// feedAudio は、鳴っている位置 now の少し先までの音をデコードし、メインスレッドへ渡す。
// まだ時計がなければ (鳴らし始める前)、先読みの分だけ渡す。
function feedAudio(current: Loading, now: number | null): void {
  if (engine === null || current.audioPid === null || current.cutting !== null) {
    return;
  }
  const es = current.audios.get(current.audioPid);
  if (es === undefined) {
    return;
  }
  const { module, audioDecoder } = engine;
  // 時計が動く前は、渡し始めた位置から先読みの分だけ渡す。届くたびに基準をずらすと、際限なく渡してしまう。
  // 渡し始める位置は、デコードできる最初のフレームにする。デュアルモノの ES で、PCE より前のフレームから数えると、
  // PCE が遅く届いたときに、鳴らし始めに要る分を渡す前に先読みの分を使い切り、鳴らし始められなくなる。
  if (now === null && current.audioStart === null) {
    const first = es.frames.find((item) => decodable(current, item));
    if (first === undefined) {
      return;
    }
    current.audioStart = first.pts;
  }
  const until = (now ?? current.audioStart ?? 0) + params.audioAhead;
  // まだ渡していないフレームから渡す。フレームの中ほどが、渡した位置より後なら、まだ渡していない。
  const cursor = current.audioCursor;
  let index =
    cursor === null ? 0 : es.frames.findIndex((item) => item.pts + item.duration / 2 > cursor);
  if (index < 0) {
    return;
  }
  while (
    index < es.frames.length &&
    (current.audioSentUntil === null || current.audioSentUntil < until)
  ) {
    const frame = es.frames[index]!;
    index += 1;
    current.audioCursor = frame.pts + frame.duration;
    // デュアルモノの ES は、チャンネルの構成 (PCE) を読むまでデコードできない。失敗を数えないよう、PCE のあるフレームまで渡さない。
    if (!decodable(current, frame)) {
      continue;
    }
    // デュアルモノでないフレームを渡すと、デコーダはその構成に替わるので、次のデュアルモノは PCE から渡し直す。
    current.pceSeen = frame.channels === 0;
    const pointer = ensureInput(frame.data.length);
    module.HEAPU8.set(frame.data, pointer);
    const sent = module._dec_send(audioDecoder, pointer, frame.data.length, frame.pts) >= 0;
    judge.onAudioDecoded(sent);
    if (!sent) {
      // 壊れたフレームは飛ばす。続けて失敗したら、判定がほかの経路へ移す。
      continue;
    }
    while (module._dec_receive(audioDecoder) === 0) {
      if (module._frame_format(audioDecoder) !== sampleFormatFloatPlanar) {
        post({ type: 'error', load: current.load, message: 'unexpected audio sample format' });

        return;
      }
      const samples = module._frame_nb_samples(audioDecoder);
      const sampleRate = module._frame_sample_rate(audioDecoder);
      const channels = module._frame_channels(audioDecoder);
      const plane = (index: number) =>
        new Float32Array(
          module.HEAPU8.buffer,
          module._frame_data(audioDecoder, index),
          samples,
        ).slice();
      // 5.1ch は前の左右の 2 つだけを鳴らす (ダウンミックスは持たない)。AAC のデコーダは、前の左右を先に並べる。
      // デュアルモノは、左が主音声、右が副音声の 2ch で出るので、選んだ方に振り分ける (#2165)。
      const decoded = { left: plane(0), right: channels >= 2 ? plane(1) : null };
      const { left, right } =
        decoded.right === null
          ? { left: decoded.left, right: decoded.left.slice() }
          : frame.channels === 0
            ? routeDualMono(decoded.left, decoded.right, audioSelection.dualMono)
            : { left: decoded.left, right: decoded.right };
      const pts =
        module._frame_has_pts(audioDecoder) !== 0 ? module._frame_pts(audioDecoder) : frame.pts;
      current.audioSentUntil = pts + (samples * ptsTicksPerSecond) / sampleRate;
      post({ type: 'audio', load: current.load, pts, sampleRate, left, right }, [
        left.buffer,
        right.buffer,
      ]);
    }
  }
}

// decodable は、フレームを今のデコーダでデコードできるかどうか。デュアルモノのフレームは、PCE を読んだ後か、PCE を持つものだけ。
function decodable(current: Loading, frame: CompressedAudio): boolean {
  return frame.channels !== 0 || current.pceSeen || startsWithPce(frame.data);
}

function onVsync(time: number): void {
  const current = loading;
  if (current === null || engine === null) {
    return;
  }
  const target = pacer.onVsync(time, (perf) => clock.at(perf));
  feedAudio(current, clock.at(time));
  if (target === null) {
    return;
  }
  // hold は、デコードしたコマ 1 つを映し続ける長さ。B フレームを省いている間は、I と P だけ (3 コマに 1 つ) になる。
  const hold = (engine.queue.frameDuration ?? 3003) * (judge.skippingB ? 3 : 1);
  // デコードに回すデータが無く、映すコマも尽きた (回線が詰まった)。データが来るまで判定で数えない。
  // 配信を読み終えていれば、映し終えたので ended を送る。
  if (current.video.length === 0 && (engine.queue.newest ?? -Infinity) + hold < target) {
    if (current.eof) {
      if (!current.endedSent) {
        current.endedSent = true;
        post({ type: 'ended', load: current.load });
      }

      return;
    }
    judge.starve(performance.now());
  }
  decodeVideo(current, target);
  if (current.drained !== null && readAheadOf(current) <= params.readAhead) {
    wakeReader(current);
  }
  const choice = engine.queue.choose(target);
  if (current.shownUntil !== null) {
    judge.onLag(Math.max(0, target - current.shownUntil) / 90);
  }
  if (choice === null) {
    return;
  }
  current.shownUntil = choice.picture.pts + hold;
  engine.renderer.draw({
    slot: choice.picture.slot,
    previous: engine.queue.previous?.slot ?? null,
    interlaced: choice.picture.interlaced,
    parity: choice.parity,
    matrix: engine.matrix,
  });
  presented += 1;
  if (!current.presented) {
    current.presented = true;
    post({ type: 'presented', load: current.load });
  }
}

// present は、時計が動く前に、待ち行列の最初のコマを描いて待つ。
function present(current: Loading, pts: number): void {
  const choice = engine?.queue.choose(pts) ?? null;
  if (engine === null || choice === null) {
    return;
  }
  engine.renderer.draw({
    slot: choice.picture.slot,
    previous: null,
    interlaced: choice.picture.interlaced,
    parity: choice.parity,
    matrix: engine.matrix,
  });
  current.presented = true;
  post({ type: 'presented', load: current.load });
}

function setVisible(next: boolean): void {
  visible = next;
  const current = loading;
  if (current === null || engine === null) {
    return;
  }
  // 見えていない間に破棄した分を読めるよう、待っている読み込みに、先読みの長さを確認し直させる。
  wakeReader(current);
  if (!next) {
    // 見えていない間は映像をデコードしない。裏のタブでデコードし続けると、見ているタブが間に合わなくなる。
    // 映像をデコードしないので、間に合っているかも数えない。
    judge.hide();
    engine.queue.clear();
    trimVideo(current);

    return;
  }
  // 見えたら、最新の I フレームからデコードし直す。
  engine.module._dec_flush(engine.videoDecoder);
  const keyframe = current.video.findLastIndex((item) => item.keyframe);
  if (keyframe > 0) {
    current.video.splice(0, keyframe);
  } else if (keyframe < 0) {
    // I フレームがなければ、崩れたコマを出さないよう、次の I フレームまで待つ。
    current.video.length = 0;
    current.waitKeyframe = true;
  }
  // デコーダを空にしたので、PTS のないコマの時刻も数え直す。
  current.nextPts = null;
  current.shownUntil = null;
  pacer.reset();
  // I フレームまで飛んでデコードし直す間は、間に合っているかを数えない。
  judge.resume(performance.now());
}

function sendStats(current: Loading): void {
  if (loading !== current || engine === null) {
    return;
  }
  const sorted = [...decodeTimes].sort((a, b) => a - b);
  const stats: DecodeStats = {
    decodeMs:
      sorted.length === 0
        ? null
        : {
            median: sorted[Math.floor(sorted.length / 2)]!,
            p95: sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * 0.95))]!,
          },
    presented,
    dropped: engine.queue.dropped,
    bytesRead: current.bytesRead,
    skippingB: judge.skippingB,
  };
  post({ type: 'stats', load: current.load, stats });
}

// judgeTick は、判定に今の様子を渡し、B フレームを省くかを切り替え、間に合わなければメインスレッドへ通知する。
function judgeTick(current: Loading): void {
  if (loading !== current || engine === null) {
    return;
  }
  const { queue, module, videoDecoder } = engine;
  judge.onDropped(queue.dropped - current.droppedSeen);
  current.droppedSeen = queue.dropped;
  const frameMs = queue.frameDuration === null ? null : queue.frameDuration / 90;
  for (const verdict of judge.tick(performance.now(), frameMs)) {
    applyVerdict(current, verdict, module, videoDecoder);
  }
}

function applyVerdict(
  current: Loading,
  verdict: Verdict,
  module: DecoderModule,
  videoDecoder: number,
): void {
  switch (verdict.type) {
    case 'skipB':
      module._dec_skip_b(videoDecoder, verdict.on ? 1 : 0);
      break;
    case 'fallback':
      post({ type: 'fallback', load: current.load, reason: verdict.reason });
      break;
  }
}

// includes は、data の中に pattern があるかを返す。
function includes(data: Uint8Array, pattern: readonly number[]): boolean {
  outer: for (let i = 0; i + pattern.length <= data.length; i++) {
    for (let j = 0; j < pattern.length; j++) {
      if (data[i + j] !== pattern[j]) {
        continue outer;
      }
    }

    return true;
  }

  return false;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
