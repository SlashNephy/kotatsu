// ブラウザ内でデコードする経路 (#1516) の、メインスレッドと worker の間のメッセージ。
// 時刻は、どれも伸ばした PTS (ptsTimeline.ts、90kHz) で受け渡す。読み込みごとに番号 (load) を振り、
// 古い読み込みのメッセージは、受け取った側で捨てる (選局したあとに、前の局の音や字幕が混ざらないように)。

import type { TsStream } from './tsDemux';

// ToDecodeWorker は、メインスレッドから worker へのメッセージ。
export type ToDecodeWorker =
  // start は、最初に 1 度だけ送る。canvas は、DOM の canvas から transferControlToOffscreen したもの。
  // timeOrigin は、メインスレッドの performance.timeOrigin。worker の performance の時刻は worker を作った時刻が 0 なので、
  // メインスレッドから届く時刻 (clock と vsync) とそろえるのに使う。
  | { type: 'start'; canvas: OffscreenCanvas; wasmUrl: string; timeOrigin: number }
  // load は、url の TS を読み始める。前の読み込みは止める。serviceId は、選ぶ番組 (null なら最初の番組)。
  | { type: 'load'; load: number; url: string; serviceId: number | null }
  // stop は、読み込みを止め、待っているものを捨てる。
  | { type: 'stop' }
  // clock は、いま耳に届いている音の PTS と、その時の performance の時刻の組。
  | { type: 'clock'; load: number; pts: number; perf: number }
  // vsync は、映している窓の画面が切り替わった時刻 (requestAnimationFrame の時刻)。
  | { type: 'vsync'; time: number }
  // visible は、映像が見えているか。見えていない間は、映像をデコードしない。
  | { type: 'visible'; visible: boolean };

// DecodeStats は、worker が 1 秒ごとに知らせる様子。
export type DecodeStats = {
  // decodeMs は、直近の 1 コマのデコードの時間の分位 (中央値と 95 パーセンタイル)。
  decodeMs: { median: number; p95: number } | null;
  // presented は、映したフィールドの数の累計、dropped は、映さずに捨てたコマの数の累計。
  presented: number;
  dropped: number;
  // bytesRead は、読んだ TS のバイト数の累計。
  bytesRead: number;
};

// FromDecodeWorker は、worker からメインスレッドへのメッセージ。
export type FromDecodeWorker =
  // ready は、デコーダと描画の用意ができた。
  | { type: 'ready' }
  // program は、PMT から選んだ番組の音声の一覧。
  | { type: 'program'; load: number; audios: TsStream[] }
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
  // error は、読み込みかデコードか描画が続けられなくなった。load がなければ、worker の用意で失敗した。
  | { type: 'error'; load: number | null; message: string };
