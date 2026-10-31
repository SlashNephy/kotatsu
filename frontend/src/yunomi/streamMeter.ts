// streamMeter は、右の列の「情報」に出す、届いた映像のビットレートと放送の形式を集める (#828)。

// rateWindowMs は、今のビットレートを平均する長さ。
export const rateWindowMs = 3_000;
// historySeconds は、グラフに並べる秒数。
export const historySeconds = 60;

// MediaChunk は、メディアの seconds 秒ぶんに当たる bytes バイト。at は、届いた時刻 (ミリ秒)。
type MediaChunk = { at: number; bytes: number; seconds: number };

// BitrateMeter は、届いたかたまりから、ビットレートを出す。
// 読むのにかかった時間ではなく、メディアの秒数で割るので、録画を放送より速く読んでも、放送のビットレートになる。
export class BitrateMeter {
  #chunks: MediaChunk[] = [];

  add(at: number, bytes: number, seconds: number) {
    if (bytes <= 0 || seconds <= 0) {
      return;
    }
    this.#chunks.push({ at, bytes, seconds });
    // グラフの最も古い点を出すのに要る分だけ残す。
    const oldest = at - historySeconds * 1_000 - rateWindowMs;
    while (this.#chunks.length > 0 && this.#chunks[0]!.at < oldest) {
      this.#chunks.shift();
    }
  }

  reset() {
    this.#chunks = [];
  }

  // rate は、at までの rateWindowMs のうちに届いたかたまりのビットレート (bps)。かたまりがなければ null。
  rate(at: number): number | null {
    let bytes = 0;
    let seconds = 0;
    for (const chunk of this.#chunks) {
      if (at - rateWindowMs <= chunk.at && chunk.at <= at) {
        bytes += chunk.bytes;
        seconds += chunk.seconds;
      }
    }

    return seconds > 0 ? (bytes * 8) / seconds : null;
  }

  // history は、at までの historySeconds 秒の、1 秒ごとのビットレートを古い順に並べる。
  history(at: number): (number | null)[] {
    return Array.from({ length: historySeconds }, (_, i) =>
      this.rate(at - (historySeconds - 1 - i) * 1_000),
    );
  }
}

// TsReading は、ブラウザ内での変換の経路で、プレイヤーから読んだ読み込みの様子。
export type TsReading = {
  // load は、読み込みを見分ける番号。読み直すと変わり、読んだバイト数が 0 から数え直される。
  load: number;
  // bytesRead は、この読み込みで読んだ TS のバイト数。
  bytesRead: number;
  // mediaEnd は、変換を終えたメディアの終わり (秒)。
  mediaEnd: number;
  // seeking は、シークで読み直しているかどうか。
  seeking: boolean;
};

// maxChunkSeconds は、1 つのかたまりとみなすメディアの長さの上限。これより飛んだら、読み込んである先へシークしたとみなす。
const maxChunkSeconds = 10;

// nextTsChunk は、前に区切った読み base と今の読み next から、メディアが進んだ分のかたまりを返す。
// 戻り値の base は、次に使う区切り。読み直しやシークでは、かたまりを返さずに区切り直す。
export function nextTsChunk(
  base: TsReading | null,
  next: TsReading,
): { base: TsReading | null; chunk: { bytes: number; seconds: number } | null } {
  if (next.seeking) {
    return { base: null, chunk: null };
  }
  if (
    base === null ||
    base.load !== next.load ||
    next.bytesRead < base.bytesRead ||
    next.mediaEnd < base.mediaEnd ||
    next.mediaEnd - base.mediaEnd > maxChunkSeconds
  ) {
    return { base: next, chunk: null };
  }
  if (next.mediaEnd === base.mediaEnd) {
    return { base, chunk: null };
  }

  return {
    base: next,
    chunk: { bytes: next.bytesRead - base.bytesRead, seconds: next.mediaEnd - base.mediaEnd },
  };
}

// RecordingSpan は、録画 (GraphQL の Recording) のうち、平均のビットレートに使うもの。
export type RecordingSpan = {
  readonly recordedFrom: string;
  readonly recordedUntil: string | null | undefined;
  readonly gaps: ReadonlyArray<{ readonly from: string; readonly to: string }>;
  readonly files: ReadonlyArray<{ readonly kind: string; readonly size: number }>;
};

// averageBitrate は、録画の TS のセグメントの大きさの合計を、録っていた長さ (途切れた区間を除く) で割った、平均のビットレート (bps)。
// 録画中なら、now までの長さで割る。TS のセグメントがなければ null。
// ブラウザ内での変換で録画を見るときは、プレイヤーが変換よりずっと先まで読むので、読んだバイト数からは測れず、これを出す。
// セグメントの書き終えた日時は、閉じるのが遅れると録画の終わりより後になるので、長さには録画の日時を使う。
export function averageBitrate(recording: RecordingSpan, now: number): number | null {
  const segments = recording.files.filter((file) => file.kind === 'SEGMENT');
  if (segments.length === 0) {
    return null;
  }
  const bytes = segments.reduce((sum, file) => sum + file.size, 0);
  const until = recording.recordedUntil == null ? now : Date.parse(recording.recordedUntil);
  const gaps = recording.gaps.reduce(
    (sum, gap) => sum + (Date.parse(gap.to) - Date.parse(gap.from)),
    0,
  );
  const milliseconds = until - Date.parse(recording.recordedFrom) - gaps;

  return milliseconds > 0 ? (bytes * 8 * 1_000) / milliseconds : null;
}

// FrameCount は、ある時点で video が受け取ったフレームの数と、再生位置。
export type FrameCount = { frames: number; time: number };

// continuesFrames は、next が previous から途切れずに再生を続けた点かどうか。再生位置が、経った時間と速さから
// 見込むより 1 秒以上ずれたら (シーク)、フレームの数が戻ったら (読み直し)、続いていないとみなす。
export function continuesFrames(
  previous: FrameCount,
  next: FrameCount,
  elapsedSeconds: number,
  rate: number,
): boolean {
  return (
    next.frames >= previous.frames &&
    Math.abs(next.time - previous.time - elapsedSeconds * rate) <= 1
  );
}

// frameRate は、2 つの時点の間の、再生位置の進みあたりのフレームの数。進んでいないか、読み直して数え直したら null。
export function frameRate(previous: FrameCount, next: FrameCount): number | null {
  const time = next.time - previous.time;
  const frames = next.frames - previous.frames;
  if (time <= 0 || frames < 0) {
    return null;
  }

  return Math.round((frames / time) * 100) / 100;
}

// formatBitrate は、ビットレートを、1 Mbps 以上なら Mbps で、それより小さければ kbps で書く。
export function formatBitrate(bps: number): string {
  return bps >= 1_000_000
    ? `${(bps / 1_000_000).toFixed(1)} Mbps`
    : `${Math.round(bps / 1_000)} kbps`;
}

// codecName は、hls.js が返すコーデックの文字列 (RFC 6381) を、コーデックの名前にする。知らないものは、そのまま返す。
export function codecName(codec: string): string {
  if (codec.startsWith('avc1') || codec.startsWith('avc3')) {
    return 'H.264';
  }
  if (codec.startsWith('hvc1') || codec.startsWith('hev1')) {
    return 'HEVC';
  }
  if (codec.startsWith('mp4a')) {
    return 'AAC';
  }

  return codec;
}

// AudioFormatKey は、音声の形式の翻訳のキー (player.streamInfo.audioFormat.*)。
export type AudioFormatKey = 'mono' | 'dualMono' | 'stereo' | 'surround';

// audioFormatKey は、EPG の音声コンポーネントの component_type (ARIB STD-B10) を、形式の翻訳のキーにする。
// 地上波と BS で使うものだけを名前にし、ほかは null にする。
export function audioFormatKey(componentType: number): AudioFormatKey | null {
  switch (componentType) {
    case 1:
      return 'mono';
    case 2:
      return 'dualMono';
    case 3:
      return 'stereo';
    case 9:
      return 'surround';
    default:
      return null;
  }
}

// TransportCounts は、ブラウザ内での変換の経路で数えた、TS のパケットの異常の数。
export type TransportCounts = { dropped: number; errors: number; scrambled: number };

// SourcePicture は、放送の映像の大きさと走査の方式。interlaced が分からなければ null。
export type SourcePicture = { width: number; height: number; interlaced: boolean | null };

// EncodedStream は、kyusu の経路で、hls.js が読んでいる画質。
export type EncodedStream = {
  // width と height は、プレイリストに書いてある大きさ。書いていなければ 0。
  width: number;
  height: number;
  videoCodec: string | null;
  audioCodec: string | null;
  // bandwidth は、hls.js が推定した帯域 (bps)。推定できていなければ null。
  bandwidth: number | null;
};

// TsPlayerReading は、ブラウザ内での変換のプレイヤーが知らせる、読み込みの様子。
export type TsPlayerReading = TsReading & {
  transport: TransportCounts | null;
  source: SourcePicture | null;
};

// HlsSegment は、kyusu の経路で、hls.js が読み終えたセグメント。
export type HlsSegment = { bytes: number; seconds: number; encoded: EncodedStream };

// StreamMeter は、再生している経路から届く様子を溜める。画面は、これを 1 秒ごとに読む。
export class StreamMeter {
  readonly bitrate = new BitrateMeter();
  transport: TransportCounts | null = null;
  source: SourcePicture | null = null;
  encoded: EncodedStream | null = null;
  #base: TsReading | null = null;

  feedTs(at: number, reading: TsPlayerReading) {
    const { base, chunk } = nextTsChunk(this.#base, reading);
    this.#base = base;
    if (chunk !== null) {
      this.bitrate.add(at, chunk.bytes, chunk.seconds);
    }
    this.transport = reading.transport;
    this.source = reading.source;
  }

  feedSegment(at: number, segment: HlsSegment) {
    this.bitrate.add(at, segment.bytes, segment.seconds);
    this.encoded = segment.encoded;
  }

  // reset は、経路を変えたときに、それまでの様子を捨てる。
  reset() {
    this.bitrate.reset();
    this.transport = null;
    this.source = null;
    this.encoded = null;
    this.#base = null;
  }
}
