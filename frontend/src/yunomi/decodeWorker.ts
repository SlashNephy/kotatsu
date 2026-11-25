/// <reference lib="webworker" />
// ブラウザ内でデコードする経路 (#1516、設計書 7.2) の worker。
// TS を読んで PES に分け (tsDemux)、映像と音声を WASM の FFmpeg でデコードし、映像は OffscreenCanvas に WebGL2 で描き
// (pictureRenderer)、音声はメインスレッドへ渡す。時計は、メインスレッドが鳴らしている音 (decodeClock) で、
// コマを出す間合いは、メインスレッドから届く画面の切り替わりの時刻で決める (fieldPacer)。
// 間に合っているかは decodeJudge で判定し、少し遅れたら B フレームを省き、間に合わなければメインスレッドへ通知する。
import createDecoderModule, { type DecoderModule } from '@kotatsu/ffmpeg-decoder';

import { AdtsSplitter } from './adtsFrames';
import { DecodeClock } from './decodeClock';
import { DecodeJudge, type Verdict } from './decodeJudge';
import type { DecodeStats, FromDecodeWorker, ToDecodeWorker } from './decodeProtocol';
import { FieldPacer } from './fieldPacer';
import { PictureQueue } from './pictureQueue';
import { type ColorMatrix, type PictureSlot, PictureRenderer } from './pictureRenderer';
import { PtsTimeline, ptsTicksPerSecond } from './ptsTimeline';
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
type CompressedAudio = { pts: number; data: Uint8Array };

type Loading = {
  load: number;
  abort: AbortController;
  demuxer: TsDemuxer;
  timeline: PtsTimeline;
  adts: AdtsSplitter;
  video: CompressedVideo[];
  audio: CompressedAudio[];
  origin: number | null;
  // audioSentUntil は、渡した音の終わりの PTS。
  audioSentUntil: number | null;
  presented: boolean;
  shape: string | null;
  bytesRead: number;
  // waitKeyframe は、I フレームから始まる PES まで映像を破棄するか (読み始め、見えるようになった直後)。
  waitKeyframe: boolean;
  // audioPid は、鳴らす音声の ES の PID。PMT の最初の音声。ほかの音声の ES は破棄する。
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
    adts: new AdtsSplitter(),
    video: [],
    audio: [],
    origin: null,
    audioSentUntil: null,
    presented: false,
    shape: null,
    bytesRead: 0,
    waitKeyframe: true,
    nextPts: null,
    shownUntil: null,
    droppedSeen: 0,
    audioPid: null,
    audioStart: null,
    lastVideoPts: null,
    demuxer: new TsDemuxer({
      serviceId,
      onProgram: (program) => {
        // 音声は、PMT の最初の ES を鳴らす (選び直しとデュアルモノの振り分けは、まだ持たない)。
        // 複数の ES を 1 つのデコーダに混ぜると、音が壊れる。
        const pid = program.audios[0]?.pid ?? null;
        if (current.audioPid !== pid) {
          current.audioPid = pid;
          current.adts.reset();
          current.audio = [];
          if (engine !== null) {
            engine.module._dec_flush(engine.audioDecoder);
          }
        }
        post({ type: 'program', load, audios: program.audios });
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

// streamPath は、tenban のライブ視聴の TS (GET /internal/streams/services/{id}) の URL。
const streamPath = /^\/internal\/streams\/services\/(\d+)(?:\?(.*))?$/;

// streamEndpoint は、url がライブ視聴の TS なら、読みに行く URL を返す。ほかの先なら null を返す。
function streamEndpoint(url: string): string | null {
  const match = streamPath.exec(url);
  if (match === null) {
    return null;
  }
  const [, id = '', query] = match;
  const path = `/internal/streams/services/${encodeURIComponent(id)}`;

  return query === undefined ? path : `${path}?${query}`;
}

async function read(current: Loading, url: string): Promise<void> {
  try {
    // 受け取った URL をそのまま読まずに、ライブ視聴の TS のパスへ組み直し、ほかのホストやパスを読まないようにする。
    const endpoint = streamEndpoint(url);
    if (endpoint === null) {
      throw new Error('the stream is not a live stream of tenban');
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
    for (;;) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      current.bytesRead += value.length;
      current.demuxer.push(value);
    }
    current.demuxer.flush();
    if (loading === current) {
      post({ type: 'ended', load: current.load });
    }
  } catch (error) {
    if (!current.abort.signal.aborted) {
      post({ type: 'error', load: current.load, message: errorMessage(error) });
    }
  }
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
    case 'audio':
      if (pes.pid !== current.audioPid) {
        return;
      }
      for (const frame of current.adts.push(pes.data, pts)) {
        // ADTS のフレームを切り出せたときだけ、音が届いたとみなす (ADTS でない内容なら、音が来ていない)。
        judge.onAudio();
        current.audio.push({ pts: frame.pts, data: frame.data });
      }
      trimAudio(current);
      feedAudio(current, clock.at(mainNow()));
      break;
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

// trimAudio は、時計が動く前 (鳴らし始める前) に、渡していない音が上限を超えたら、古い方から破棄する。
// 音ありの自動再生を断られている間に、バッファし続けないため。
function trimAudio(current: Loading): void {
  if (clock.at(mainNow()) !== null) {
    return;
  }
  const newest = current.audio.at(-1)?.pts ?? null;
  while (
    newest !== null &&
    current.audio.length > 0 &&
    newest - current.audio[0]!.pts > params.maxCompressed
  ) {
    current.audio.shift();
  }
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
  if (engine === null) {
    return;
  }
  const { module, audioDecoder } = engine;
  // 時計が動く前は、渡し始めた位置から先読みの分だけ渡す。届くたびに基準をずらすと、際限なく渡してしまう。
  if (now === null && current.audioStart === null) {
    current.audioStart = current.audio[0]?.pts ?? null;
  }
  const until = (now ?? current.audioStart ?? 0) + params.audioAhead;
  while (
    current.audio.length > 0 &&
    (current.audioSentUntil === null || current.audioSentUntil < until)
  ) {
    const frame = current.audio.shift()!;
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
      const left = plane(0);
      const right = channels >= 2 ? plane(1) : left.slice();
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
  if (current.video.length === 0 && (engine.queue.newest ?? -Infinity) + hold < target) {
    judge.starve(performance.now());
  }
  decodeVideo(current, target);
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
