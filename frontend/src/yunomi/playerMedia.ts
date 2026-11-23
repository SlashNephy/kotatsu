import { sharedAudioContext } from './sharedAudioContext';

// PlayerMedia は、再生している映像と音を、プレイヤーの部品から扱うための面 (#1516)。
// 今の経路 (ブラウザ内での変換と kyusu) は <video> で鳴らすが、MPEG-2 を WASM でデコードして canvas に描く経路は
// <video> を持たない。部品が <video> に直に触ると、その経路で動かせないので、部品はこの面を使う。
//
// 面の形は <video> に合わせてあるが、<video> の経路と canvas の経路で違うものは、次のように分けている。
// - シーク: currentTime は読むだけにし、動かすときは seek を呼ぶ。どこまで動けるかは、読み込みごとに seekMode で確かめる。
// - 音: <video> の音を WebAudio に繋ぐ (createMediaElementSource) 代わりに、audioPort で、音の出どころと行き先を受け取る。
// - 画面: キャプチャ (drawImage) に渡すものと、DOM に置く要素は、<video> とは限らない。frame と element で受け取る。
// - 作れない型: MediaError、TimeRanges、VideoPlaybackQuality は JS で作れないので、同じ形の型で受け取る。
//
// ライブの追いつき (liveCatchUp)、映像だけの遅れの検出 (useVideoLag)、handover の前送り (handoverAdvance) は、
// <video> の経路のための処理で、canvas の経路では経路の中で行う。そのため、この面を通さず、video で <video> を受け取る
// (<video> を持たない経路では null になり、これらの処理は動かない)。

// PlayerMediaEventType は、PlayerMedia が送るイベント。<video> の同じ名前のイベントと同じ時に送る。
export type PlayerMediaEventType =
  | 'canplay'
  | 'durationchange'
  | 'emptied'
  | 'ended'
  | 'error'
  | 'loadeddata'
  | 'loadstart'
  | 'pause'
  | 'play'
  | 'playing'
  | 'resize'
  | 'seeked'
  | 'seeking'
  | 'timeupdate'
  | 'volumechange'
  | 'waiting';

// PlayerMediaSeekMode は、seek でどこまで正確に動けるか。
// - exact: 指定した位置へ動く。
// - keyframe: 指定した位置以降の、最初に映せるコマ (I フレーム) へ動く。前へは戻らない。
// - none: 動けない (ライブなど)。
export type PlayerMediaSeekMode = 'exact' | 'keyframe' | 'none';

// PlayerMediaError は、再生のエラー。MediaError と同じ形。
export type PlayerMediaError = {
  readonly code: number;
  readonly message: string;
};

// PlayerMediaRanges は、時刻の範囲の並び。TimeRanges と同じ形。
export type PlayerMediaRanges = {
  readonly length: number;
  start(index: number): number;
  end(index: number): number;
};

// PlayerMediaQuality は、出したコマと落としたコマの数。VideoPlaybackQuality のうち、部品が読むもの。
export type PlayerMediaQuality = {
  readonly totalVideoFrames: number;
  readonly droppedVideoFrames: number;
};

// PlayerFrameMetadata は、コマを出したときに受け取るもの。VideoFrameCallbackMetadata のうち、部品が読むもの。
export type PlayerFrameMetadata = {
  // mediaTime は、出したコマの位置 (秒)。
  readonly mediaTime: number;
};

export type PlayerFrameCallback = (now: DOMHighResTimeStamp, metadata: PlayerFrameMetadata) => void;

// AudioPort は、経路の音を WebAudio に通すための口。source から sink までの間に、音量や補正の段を挟む。
export type AudioPort = {
  // context は、source と sink のある AudioContext。sharedAudioContext と同じもの。
  readonly context: AudioContext;
  // source は、経路の音。
  readonly source: AudioNode;
  // sink は、段を挟んだ後の音を繋ぐ先。
  readonly sink: AudioNode;
};

type Listener = EventListenerOrEventListenerObject;

export interface PlayerMedia {
  // video は、<video> で鳴らす経路の <video>。<video> の経路のための処理 (先頭の説明) だけが使う。
  // <video> を持たない経路では null で、それらの処理は動かない。
  readonly video: HTMLVideoElement | null;
  // currentTime は、再生している位置 (秒)。
  readonly currentTime: number;
  // duration は、長さ (秒)。終わりの決まっていないライブでは Infinity か NaN。
  readonly duration: number;
  readonly paused: boolean;
  readonly seeking: boolean;
  readonly ended: boolean;
  // readyState は、HTMLMediaElement の readyState と同じ段を返す。
  readonly readyState: number;
  readonly error: PlayerMediaError | null;
  readonly buffered: PlayerMediaRanges;
  // videoWidth と videoHeight は、画素の縦横比を掛けた後の、映す大きさ (1440x1080 なら 1920x1080)。
  readonly videoWidth: number;
  readonly videoHeight: number;
  // volume は、0 から 1。100% を超える分は、audioPort の先の段で上げる (volumeBoost)。
  volume: number;
  muted: boolean;
  playbackRate: number;
  defaultPlaybackRate: number;
  // seekMode は、今読み込んでいるものを、seek でどこまで正確に動けるか。読み込むたびに変わることがある。
  readonly seekMode: PlayerMediaSeekMode;
  play(): Promise<void>;
  pause(): void;
  // seek は、再生の位置を seconds 秒へ動かす。seekMode が none のときは呼ばない。
  seek(seconds: number): void;
  // requestVideoFrameCallback と cancelVideoFrameCallback は、コマを出すたびに呼び返す。
  // 経路が対応していなければ無い (有無で、requestAnimationFrame などに切り替える)。
  readonly requestVideoFrameCallback?: (callback: PlayerFrameCallback) => number;
  readonly cancelVideoFrameCallback?: (handle: number) => void;
  // getVideoPlaybackQuality は、出したコマと落としたコマの数を返す。経路が対応していなければ無い。
  readonly getVideoPlaybackQuality?: () => PlayerMediaQuality;
  // audioPort は、音を WebAudio に通すための口を返す。初めて呼ぶと、経路の音は WebAudio を通るようになり、
  // sink に繋ぐまで聞こえなくなる。
  audioPort(): AudioPort;
  // frame は、今のコマを描いたもの。drawImage に渡す。すぐに描けない経路では null。
  readonly frame: CanvasImageSource | null;
  // element は、映像を映している DOM の要素。
  readonly element: HTMLElement;
  addEventListener(
    type: PlayerMediaEventType,
    listener: Listener,
    options?: AddEventListenerOptions | boolean,
  ): void;
  removeEventListener(
    type: PlayerMediaEventType,
    listener: Listener,
    options?: EventListenerOptions | boolean,
  ): void;
}

// VideoElementMedia は、<video> で鳴らす経路の PlayerMedia。<video> の同じ名前のものに、そのまま渡す。
// イベントも <video> に付けるので、受け取るイベントの target は <video> になる (部品は target を読まない)。
// 同じ <video> に 2 つ作ると、音の口 (createMediaElementSource は 1 つの <video> に 1 度だけ) が分かれるので、
// mediaOf からだけ作る。
class VideoElementMedia implements PlayerMedia {
  readonly seekMode = 'exact';
  readonly requestVideoFrameCallback?: (callback: PlayerFrameCallback) => number;
  readonly cancelVideoFrameCallback?: (handle: number) => void;
  readonly getVideoPlaybackQuality?: () => PlayerMediaQuality;
  private port: AudioPort | null = null;

  constructor(readonly video: HTMLVideoElement) {
    // 対応していないブラウザでは、メソッドを持たせない。部品は有無を見て、別の方法に切り替えるため。
    if (typeof video.requestVideoFrameCallback === 'function') {
      this.requestVideoFrameCallback = (callback) => video.requestVideoFrameCallback(callback);
      this.cancelVideoFrameCallback = (handle) => video.cancelVideoFrameCallback(handle);
    }
    if (typeof video.getVideoPlaybackQuality === 'function') {
      this.getVideoPlaybackQuality = () => video.getVideoPlaybackQuality();
    }
  }

  get currentTime(): number {
    return this.video.currentTime;
  }

  get duration(): number {
    return this.video.duration;
  }

  get paused(): boolean {
    return this.video.paused;
  }

  get seeking(): boolean {
    return this.video.seeking;
  }

  get ended(): boolean {
    return this.video.ended;
  }

  get readyState(): number {
    return this.video.readyState;
  }

  get error(): PlayerMediaError | null {
    return this.video.error;
  }

  get buffered(): PlayerMediaRanges {
    return this.video.buffered;
  }

  get videoWidth(): number {
    return this.video.videoWidth;
  }

  get videoHeight(): number {
    return this.video.videoHeight;
  }

  get volume(): number {
    return this.video.volume;
  }

  set volume(value: number) {
    this.video.volume = value;
  }

  get muted(): boolean {
    return this.video.muted;
  }

  set muted(value: boolean) {
    this.video.muted = value;
  }

  get playbackRate(): number {
    return this.video.playbackRate;
  }

  set playbackRate(value: number) {
    this.video.playbackRate = value;
  }

  get defaultPlaybackRate(): number {
    return this.video.defaultPlaybackRate;
  }

  set defaultPlaybackRate(value: number) {
    this.video.defaultPlaybackRate = value;
  }

  get frame(): CanvasImageSource {
    return this.video;
  }

  get element(): HTMLElement {
    return this.video;
  }

  play(): Promise<void> {
    return this.video.play();
  }

  pause(): void {
    this.video.pause();
  }

  seek(seconds: number): void {
    this.video.currentTime = seconds;
  }

  audioPort(): AudioPort {
    if (this.port === null) {
      const context = sharedAudioContext();
      this.port = {
        context,
        source: context.createMediaElementSource(this.video),
        sink: context.destination,
      };
    }

    return this.port;
  }

  addEventListener(
    type: PlayerMediaEventType,
    listener: Listener,
    options?: AddEventListenerOptions | boolean,
  ): void {
    this.video.addEventListener(type, listener, options);
  }

  removeEventListener(
    type: PlayerMediaEventType,
    listener: Listener,
    options?: EventListenerOptions | boolean,
  ): void {
    this.video.removeEventListener(type, listener, options);
  }
}

// medias は、<video> ごとの PlayerMedia。同じ <video> には、同じものを返す (WeakMap の鍵や、React の依存で比べるため)。
const medias = new WeakMap<HTMLVideoElement, PlayerMedia>();

// mediaOf は、video の PlayerMedia を返す。
export function mediaOf(video: HTMLVideoElement): PlayerMedia {
  let media = medias.get(video);
  if (media === undefined) {
    media = new VideoElementMedia(video);
    medias.set(video, media);
  }

  return media;
}
