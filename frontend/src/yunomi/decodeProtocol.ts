// ブラウザ内でデコードする経路 (#1516) の、メインスレッドと worker の間のメッセージ。
// 時刻は、どれも伸ばした PTS (ptsTimeline.ts、90kHz) で受け渡す。読み込みごとに番号 (load) を振り、
// 古い読み込みのメッセージは、受け取った側で破棄する (選局したあとに、前の局の音や字幕が混ざらないように)。

import type { DualMonoChannel } from './decodeAudio';
import type { FallbackReason } from './decodeJudge';
import type { SuperResolutionMethod, SuperResolutionStrength } from './pictureUpscaler';

// ToDecodeWorker は、メインスレッドから worker へのメッセージ。
export type ToDecodeWorker =
  // start は、最初に 1 度だけ送る。canvas は、DOM の canvas から transferControlToOffscreen したもの。
  // timeOrigin は、メインスレッドの performance.timeOrigin。worker の performance の時刻は worker を作った時刻が 0 なので、
  // メインスレッドから届く時刻 (clock と vsync) とそろえるのに使う。
  | { type: 'start'; canvas: OffscreenCanvas; wasmUrl: string; timeOrigin: number }
  // load は、url の TS を読み始める。前の読み込みは止める。serviceId は、選ぶ番組 (null なら最初の番組)。
  | { type: 'load'; load: number; url: string; serviceId: number | null }
  // stop は、読み込みを止め、待っているものを破棄する。
  | { type: 'stop' }
  // clock は、いま耳に届いている音の PTS と、その時の performance の時刻の組。
  | { type: 'clock'; load: number; pts: number; perf: number }
  // vsync は、映しているウィンドウの画面が切り替わった時刻 (requestAnimationFrame の時刻)。
  | { type: 'vsync'; time: number }
  // visible は、映像が見えているか。見えていない間は、映像をデコードしない。
  | { type: 'visible'; visible: boolean }
  // selectAudio は、鳴らす音声を選ぶ (#2165)。pid は鳴らす ES、dualMono はデュアルモノの ES の振り分け。
  // 省いた方は、今のままにする。選んだものは、読み込み直しても保つ。
  | { type: 'selectAudio'; pid?: number; dualMono?: DualMonoChannel }
  // audioFrom は、audioCut への返事。pts から後に予約した音を破棄したので、pts から渡し直してほしい (#2165)。
  // pts が null なら、まだ鳴らし始めていないので、貯めた音をすべて破棄した。渡し始めた位置から渡し直してほしい。
  // cut は、audioCut の番号。続けて選び直したときに、前の切り替えの返事と見分ける。
  | { type: 'audioFrom'; load: number; cut: number; pts: number | null }
  // display は、コマを映す大きさと、映像の拡大 (#1654、#2276) と擬似 HDR (#1656) の設定。大きさか設定が変わるたびに送る。
  // worker は、画面の大きさも端末ごとの設定も読めないので、メインスレッドが計測して渡す。
  | ({ type: 'display' } & DecodeDisplay);

// DecodeDisplay は、コマを映す大きさと、映像の拡大と擬似 HDR の設定。
export type DecodeDisplay = {
  // box は、絵を枠に収めた (object-fit: contain) 矩形の、端末の画素での大きさ。計測できていなければ null。
  box: { width: number; height: number } | null;
  // zoom は、切り抜き (#496) で掛けている CSS の拡大率。
  zoom: { x: number; y: number };
  superResolution: boolean;
  strength: SuperResolutionStrength;
  method: SuperResolutionMethod;
  // hdr は、擬似 HDR (#1656) で描くか。端末ごとの設定で入れていて、ブラウザが対応し、画面が HDR を出せるときだけ true。
  hdr: boolean;
};

// DecodeAudioStream は、番組の音声の ES。dualMono は、届いた ES がデュアルモノ (channel_configuration が 0) だったか。
export type DecodeAudioStream = { pid: number; componentTag: number | null; dualMono: boolean };

// DecodeStats は、worker が 1 秒ごとに通知する様子。
export type DecodeStats = {
  // decodeMs は、直近の 1 コマのデコードの時間の分位 (中央値と 95 パーセンタイル)。
  decodeMs: { median: number; p95: number } | null;
  // presented は、映したフィールドの数の累計、dropped は、映さずに破棄したコマの数の累計。
  presented: number;
  dropped: number;
  // bytesRead は、読んだ TS のバイト数の累計。
  bytesRead: number;
  // skippingB は、間に合わないために B フレームのデコードを省いているか。
  skippingB: boolean;
};

// FromDecodeWorker は、worker からメインスレッドへのメッセージ。
export type FromDecodeWorker =
  // ready は、デコーダと描画の用意ができた。
  | { type: 'ready' }
  // audioTracks は、番組の音声の ES と、鳴らしている ES (current) とデュアルモノの振り分け (#2165)。
  // 番組の構成が変わったとき、ES がデュアルモノと分かったとき、選び直したときに送る。
  | {
      type: 'audioTracks';
      load: number;
      streams: DecodeAudioStream[];
      current: number | null;
      dualMono: DualMonoChannel;
    }
  // audioCut は、鳴らす音声を選び直したので、少し先から後に予約した音を破棄させる (#2165)。
  // どこから破棄したかは、メインスレッドが audioFrom で返す。予約を持つメインスレッドでないと、間に合う位置を決められない。
  | { type: 'audioCut'; load: number; cut: number }
  // origin は、最初に映すコマの PTS。画面の時刻 0 に当たる (currentTime = (時計 − origin) / 90000)。
  | { type: 'origin'; load: number; pts: number }
  // picture は、映す大きさ (画素の縦横比を掛けた後) と走査の方式が変わった。
  | { type: 'picture'; load: number; width: number; height: number; interlaced: boolean }
  // audio は、デコードした音 (前の左右の 2ch)。pts は、最初のサンプルの時刻。
  | {
      type: 'audio';
      load: number;
      pts: number;
      sampleRate: number;
      left: Float32Array;
      right: Float32Array;
    }
  // caption は、字幕か文字スーパーの PES のペイロード。
  | {
      type: 'caption';
      load: number;
      kind: 'captions' | 'superimpose';
      pts: number;
      data: ArrayBuffer;
    }
  // presented は、最初のコマを描いた (読み込み中の表示を外せる)。
  | { type: 'presented'; load: number }
  // ended は、TS を読み終えた (ライブでは、配信が切れた)。
  | { type: 'ended'; load: number }
  | { type: 'stats'; load: number; stats: DecodeStats }
  // fallback は、この端末では間に合わない (slow) か、音が届かないかデコードできない (audio)。ほかの経路へ移してほしい。
  | { type: 'fallback'; load: number; reason: FallbackReason }
  // error は、読み込みかデコードか描画が続けられなくなった。load がなければ、worker の用意で失敗した。
  | { type: 'error'; load: number | null; message: string };
