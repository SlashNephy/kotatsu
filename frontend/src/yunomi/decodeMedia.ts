import { AudioTimeline } from './audioTimeline';
import type { DecodeStats, FromDecodeWorker, ToDecodeWorker } from './decodeProtocol';
import type {
  AudioPort,
  PlayerMedia,
  PlayerMediaError,
  PlayerMediaEventType,
  PlayerMediaQuality,
  PlayerMediaRanges,
} from './playerMedia';
import { ptsTicksPerSecond } from './ptsTimeline';
import { sharedAudioContext } from './sharedAudioContext';

// ブラウザ内でデコードする経路 (#1516、設計書 7.2) の PlayerMedia。
// 映像は worker が canvas (OffscreenCanvas) に描き、音はここで Web Audio に予約して鳴らす。時計は、鳴っている音の PTS で、
// それを worker へ送り、worker が映すコマを選ぶ。
//
// <video> と違う所:
// - シークできない (seekMode は none)。速さは等速だけ (playbackRate は 1 のまま)。
// - ライブで止めてから再開すると、放送の今から読み直す (止めている間の音と映像を貯めないため)。読み直すと、
//   <video> と同じく emptied と loadstart を送る。
// - 音ありの自動再生を断られたら、<video> と同じく止まったままにし、play() を NotAllowedError で断る。

// readyStates は、HTMLMediaElement の readyState の段。
const readyStates = { nothing: 0, currentData: 2, enoughData: 4 } as const;

// startBuffer は、鳴らし始めるまでに貯める音の長さ (PTS の単位)。
const startBuffer = 0.3 * ptsTicksPerSecond;
// timeupdateInterval は、timeupdate を送る間隔 (ミリ秒)。<video> と同じく、4 回/秒ほどにする。
const timeupdateInterval = 250;
// staleStart は、鳴らし始める前に溜めた音が、これより古くなったら (ミリ秒)、鳴らし始めるときに放送の今から読み直す。
// 音ありの自動再生を断られたまま待っていた間の音を、遅れて鳴らさないため。
const staleStart = 3_000;
// resumeTimeout は、AudioContext を動かすのを待つ上限 (ミリ秒)。ユーザーの操作の前は、返らないブラウザがある。
const resumeTimeout = 300;
// visibilityInterval は、映像が見えているかを調べる間隔 (ミリ秒)。別のウィンドウへ移ると文書が替わるので、
// イベントではなく、映している文書をその都度調べる。
const visibilityInterval = 500;

class MediaFailure implements PlayerMediaError {
  constructor(
    readonly code: number,
    readonly message: string,
  ) {}
}

// DecodeMediaOptions は、DecodeMedia を作るのに要るもの。
export type DecodeMediaOptions = {
  canvas: HTMLCanvasElement;
  worker: Worker;
};

export class DecodeMedia extends EventTarget implements PlayerMedia {
  readonly video = null;
  readonly seekMode = 'none';
  readonly duration = Number.POSITIVE_INFINITY;
  readonly element: HTMLCanvasElement;
  private readonly worker: Worker;
  private readonly context: AudioContext;
  // volumeNode は、音量とミュートを当てる。音の口 (audioPort) の source でもある。
  private readonly volumeNode: GainNode;
  private readonly timeline = new AudioTimeline();
  // pending は、鳴らし始める前に貯めている音。
  private pending: { pts: number; sampleRate: number; left: Float32Array; right: Float32Array }[] =
    [];
  private sources = new Set<AudioBufferSourceNode>();
  private load = 0;
  private origin: number | null = null;
  private width = 0;
  private height = 0;
  private stats: DecodeStats | null = null;
  // animation は、requestAnimationFrame の番号。
  private animation: number | null = null;
  private lastTimeupdate = 0;
  // stopped は、止めたときに読み込みも止めたか。再開したら、放送の今から読み直す。
  private stopped = false;
  // pendingSince は、鳴らし始める前に、最初の音が届いた時刻。
  private pendingSince: number | null = null;
  private visible: boolean | null = null;
  private visibilityTimer: ReturnType<typeof setInterval> | null = null;
  private stateValue = {
    paused: true,
    ended: false,
    readyState: readyStates.nothing as number,
    error: null as PlayerMediaError | null,
    volume: 1,
    muted: false,
    waiting: false,
  };
  private url: string | null = null;
  private serviceId: number | null = null;

  constructor({ canvas, worker }: DecodeMediaOptions) {
    super();
    this.element = canvas;
    this.worker = worker;
    this.context = sharedAudioContext();
    this.volumeNode = this.context.createGain();
    // 音量の段 (volumeBoost) が段を挟むまでは、そのまま鳴らす。
    this.volumeNode.connect(this.context.destination);
    worker.addEventListener('message', this.onMessage);
  }

  get currentTime(): number {
    const pts = this.clockPts();
    if (pts === null || this.origin === null) {
      return 0;
    }

    return Math.max(0, (pts - this.origin) / ptsTicksPerSecond);
  }

  get paused(): boolean {
    return this.stateValue.paused;
  }

  get seeking(): boolean {
    return false;
  }

  get ended(): boolean {
    return this.stateValue.ended;
  }

  get readyState(): number {
    return this.stateValue.readyState;
  }

  get error(): PlayerMediaError | null {
    return this.stateValue.error;
  }

  get buffered(): PlayerMediaRanges {
    const end = this.timeline.bufferedUntil;
    if (end === null || this.origin === null) {
      return ranges([]);
    }

    return ranges([
      [this.currentTime, Math.max(this.currentTime, (end - this.origin) / ptsTicksPerSecond)],
    ]);
  }

  get videoWidth(): number {
    return this.width;
  }

  get videoHeight(): number {
    return this.height;
  }

  get volume(): number {
    return this.stateValue.volume;
  }

  set volume(value: number) {
    const next = Math.min(Math.max(value, 0), 1);
    if (next === this.stateValue.volume) {
      return;
    }
    this.stateValue.volume = next;
    this.applyVolume();
    this.fire('volumechange');
  }

  get muted(): boolean {
    return this.stateValue.muted;
  }

  set muted(value: boolean) {
    if (value === this.stateValue.muted) {
      return;
    }
    this.stateValue.muted = value;
    this.applyVolume();
    this.fire('volumechange');
  }

  // playbackRate と defaultPlaybackRate は、等速のまま変えない (自前で鳴らす音を速めると音程が変わるため)。
  get playbackRate(): number {
    return 1;
  }

  set playbackRate(_value: number) {}

  get defaultPlaybackRate(): number {
    return 1;
  }

  set defaultPlaybackRate(_value: number) {}

  // frame は、worker が描いた canvas。drawImage で、描いた最後のコマを写せる (#1516 で確かめた)。
  get frame(): CanvasImageSource {
    return this.element;
  }

  getVideoPlaybackQuality = (): PlayerMediaQuality => ({
    totalVideoFrames: (this.stats?.presented ?? 0) + (this.stats?.dropped ?? 0),
    droppedVideoFrames: this.stats?.dropped ?? 0,
  });

  // currentLoad は、今の読み込みの番号。古い読み込みのメッセージ (字幕など) を見分けるのに使う。
  get currentLoad(): number {
    return this.load;
  }

  // open は、url の TS を読み始める。serviceId は、選ぶ番組。
  open(url: string, serviceId: number | null): void {
    this.url = url;
    this.serviceId = serviceId;
    this.restart();
  }

  async play(): Promise<void> {
    if (this.context.state !== 'running') {
      try {
        // ユーザーの操作の直後なら、動き出すまで待つ。操作の前は、返らないブラウザがあるので、待つ時間に上限を付ける。
        await (navigator.userActivation?.isActive === true
          ? this.context.resume()
          : Promise.race([
              this.context.resume(),
              new Promise((resolve) => setTimeout(resolve, resumeTimeout)),
            ]));
      } catch {
        // 下で断る。
      }
    }
    if (this.context.state !== 'running') {
      // ユーザーが操作するまで、音ありで鳴らせない。<video> と同じく断る。
      throw new DOMException('the user has not interacted with the page yet', 'NotAllowedError');
    }
    if (!this.stateValue.paused) {
      return;
    }
    this.stateValue.paused = false;
    this.fire('play');
    const stale = this.pendingSince !== null && performance.now() - this.pendingSince > staleStart;
    if (this.stopped || stale) {
      // 止めている間に捨てたか、鳴らし始める前に溜めた音が古い。放送の今から読み直す。
      this.restart();

      return;
    }
    this.startAudio();
  }

  pause(): void {
    if (this.stateValue.paused) {
      return;
    }
    this.stateValue.paused = true;
    // 止めている間の音と映像は貯めない。再開したら、放送の今から読み直す。
    this.stopAudio();
    this.worker.postMessage({ type: 'stop' } satisfies ToDecodeWorker);
    this.stopped = true;
    this.fire('pause');
  }

  seek(_seconds: number): void {
    // seekMode が none なので、呼ばれない。
  }

  audioPort(): AudioPort {
    return { context: this.context, source: this.volumeNode, sink: this.context.destination };
  }

  // dispose は、音を止め、worker から外れる。worker は、作った側が止める。
  dispose(): void {
    this.stopAudio();
    this.worker.removeEventListener('message', this.onMessage);
    this.volumeNode.disconnect();
    if (this.visibilityTimer !== null) {
      clearInterval(this.visibilityTimer);
      this.visibilityTimer = null;
    }
    if (this.animation !== null) {
      (this.element.ownerDocument.defaultView ?? window).cancelAnimationFrame(this.animation);
      this.animation = null;
    }
  }

  // restart は、新しい読み込みを始める。
  private restart(): void {
    if (this.url === null) {
      return;
    }
    this.load += 1;
    this.stopped = false;
    this.pendingSince = null;
    this.watchVisibility();
    this.stopAudio();
    this.origin = null;
    this.stats = null;
    this.stateValue.ended = false;
    this.stateValue.error = null;
    this.stateValue.readyState = readyStates.nothing;
    this.fire('emptied');
    this.fire('loadstart');
    this.worker.postMessage({
      type: 'load',
      load: this.load,
      url: this.url,
      serviceId: this.serviceId,
    } satisfies ToDecodeWorker);
    this.loop();
  }

  private readonly onMessage = (event: MessageEvent<FromDecodeWorker>) => {
    const message = event.data;
    if ('load' in message && message.load !== null && message.load !== this.load) {
      // 古い読み込みのメッセージ。
      return;
    }
    switch (message.type) {
      case 'origin':
        this.origin = message.pts;
        break;
      case 'picture':
        this.width = message.width;
        this.height = message.height;
        this.fire('resize');
        break;
      case 'presented':
        if (this.stateValue.readyState < readyStates.currentData) {
          this.stateValue.readyState = readyStates.currentData;
          this.fire('loadeddata');
        }
        break;
      case 'audio':
        this.onAudio(message);
        break;
      case 'stats':
        this.stats = message.stats;
        break;
      case 'ended':
        this.stateValue.ended = true;
        this.fire('ended');
        break;
      case 'error':
        this.stateValue.error = new MediaFailure(4, message.message);
        this.fire('error');
        break;
      default:
        break;
    }
  };

  private onAudio(message: Extract<FromDecodeWorker, { type: 'audio' }>): void {
    if (this.timeline.ptsAt(this.context.currentTime) === null) {
      this.pendingSince ??= performance.now();
      this.pending.push(message);
      const first = this.pending[0]!.pts;
      const last = message.pts + (message.left.length * ptsTicksPerSecond) / message.sampleRate;
      if (last - first >= startBuffer && this.stateValue.readyState < readyStates.enoughData) {
        this.stateValue.readyState = readyStates.enoughData;
        this.fire('canplay');
        if (!this.stateValue.paused) {
          this.startAudio();
        }
      }

      return;
    }
    this.scheduleChunk(message);
  }

  // startAudio は、貯めた音を鳴らし始める。
  private startAudio(): void {
    if (this.stateValue.paused || this.context.state !== 'running') {
      return;
    }
    const pending = this.pending;
    this.pending = [];
    for (const chunk of pending) {
      this.scheduleChunk(chunk);
    }
    if (pending.length > 0) {
      this.pendingSince = null;
      this.fire('playing');
    }
  }

  private scheduleChunk(chunk: {
    pts: number;
    sampleRate: number;
    left: Float32Array;
    right: Float32Array;
  }): void {
    if (this.stateValue.paused) {
      this.pending.push(chunk);

      return;
    }
    const duration = (chunk.left.length * ptsTicksPerSecond) / chunk.sampleRate;
    const restartsBefore = this.timeline.restarts;
    const scheduled = this.timeline.schedule(chunk.pts, duration, this.context.currentTime);
    if (scheduled === null) {
      return;
    }
    if (this.timeline.restarts !== restartsBefore) {
      // 途切れた。鳴らし直すまで待ったことにする。
      this.fire('waiting');
      this.fire('playing');
    }
    const buffer = this.context.createBuffer(2, chunk.left.length, chunk.sampleRate);
    buffer.copyToChannel(chunk.left as Float32Array<ArrayBuffer>, 0);
    buffer.copyToChannel(chunk.right as Float32Array<ArrayBuffer>, 1);
    const source = this.context.createBufferSource();
    source.buffer = buffer;
    source.connect(this.volumeNode);
    source.onended = () => this.sources.delete(source);
    source.start(scheduled.when, scheduled.offset);
    this.sources.add(source);
  }

  private stopAudio(): void {
    for (const source of this.sources) {
      source.onended = null;
      try {
        source.stop();
      } catch {
        // 鳴らし始める前でも止められるが、念のため。
      }
      source.disconnect();
    }
    this.sources.clear();
    this.pending = [];
    this.pendingSince = null;
    this.timeline.reset();
  }

  private applyVolume(): void {
    this.volumeNode.gain.value = this.stateValue.muted ? 0 : this.stateValue.volume;
  }

  // clockPts は、いま耳に届いている音の PTS。鳴らしていなければ null。
  private clockPts(): number | null {
    const stamp = this.context.getOutputTimestamp();
    const time = stamp.contextTime ?? this.context.currentTime;

    return this.timeline.ptsAt(time);
  }

  // loop は、画面の切り替わりごとに、時計と切り替わりの時刻を worker へ送る。映している窓の requestAnimationFrame を使う
  // (別のウィンドウに移したときも、その窓の切り替わりに合わせる)。
  private loop(): void {
    if (this.animation !== null) {
      return;
    }
    const tick = (time: number) => {
      this.animation = null;
      const stamp = this.context.getOutputTimestamp();
      const pts = stamp.contextTime === undefined ? null : this.timeline.ptsAt(stamp.contextTime);
      if (pts !== null && stamp.performanceTime !== undefined && !this.stateValue.paused) {
        this.worker.postMessage({
          type: 'clock',
          load: this.load,
          pts,
          perf: stamp.performanceTime,
        } satisfies ToDecodeWorker);
      }
      this.worker.postMessage({ type: 'vsync', time } satisfies ToDecodeWorker);
      if (!this.stateValue.paused && time - this.lastTimeupdate >= timeupdateInterval) {
        this.lastTimeupdate = time;
        this.fire('timeupdate');
      }
      // 別のウィンドウ (PiP) に移したときも、映している窓の切り替わりに合わせる。
      const view = this.element.ownerDocument.defaultView ?? window;
      this.animation = view.requestAnimationFrame(tick);
    };
    const view = this.element.ownerDocument.defaultView ?? window;
    this.animation = view.requestAnimationFrame(tick);
  }

  // watchVisibility は、映像が見えているかを、変わったときに worker へ知らせる。見えていない間、worker は映像をデコードしない。
  // ピクチャーインピクチャーの別のウィンドウに移していれば、その窓の文書で見る。
  private watchVisibility(): void {
    if (this.visibilityTimer !== null) {
      return;
    }
    const check = () => {
      const visible = this.element.ownerDocument.visibilityState !== 'hidden';
      if (visible !== this.visible) {
        this.visible = visible;
        this.worker.postMessage({ type: 'visible', visible } satisfies ToDecodeWorker);
      }
    };
    check();
    this.visibilityTimer = setInterval(check, visibilityInterval);
  }

  // fire は、<video> と同じく、イベントをタスクとして送る (呼んだ処理の中では送らない)。
  private fire(type: PlayerMediaEventType): void {
    setTimeout(() => this.dispatchEvent(new Event(type)), 0);
  }
}

function ranges(list: [number, number][]): PlayerMediaRanges {
  return {
    length: list.length,
    start: (index) => list[index]![0],
    end: (index) => list[index]![1],
  };
}
