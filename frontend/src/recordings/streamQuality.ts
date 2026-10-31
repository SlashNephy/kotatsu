// streamQuality は、録画の詳細の PID ごとの品質の表 (#473、#827) に出す値を作る。

// packetSize は、TS のパケットの大きさ (バイト)。
const packetSize = 188;

// packetBitrate は、packets 個のパケットを seconds 秒で割った、平均のビットレート (bps)。長さが分からなければ null。
export function packetBitrate(packets: number, seconds: number): number | null {
  return seconds > 0 ? (packets * packetSize * 8) / seconds : null;
}

// streamTypeNames は、名前で出す stream_type。ARIB の地上波、BS、CS で使う映像と音声のもの。
const streamTypeNames: Readonly<Record<number, string>> = {
  0x01: 'MPEG-1',
  0x02: 'MPEG-2',
  0x03: 'MPEG-1 Audio',
  0x04: 'MPEG-2 Audio',
  0x0f: 'AAC',
  0x11: 'AAC (LATM)',
  0x1b: 'H.264',
  0x24: 'HEVC',
};

// streamTypeName は、PMT の stream_type を、コーデックの名前にする。名前のないものは 0x06 のように書く。分からなければ null。
export function streamTypeName(streamType: number | null): string | null {
  if (streamType === null) {
    return null;
  }

  return (
    streamTypeNames[streamType] ?? `0x${streamType.toString(16).toUpperCase().padStart(2, '0')}`
  );
}

// QualityCounts は、PID ごとの数のうち、合計の行に足すもの。
export type QualityCounts = {
  readonly packets: number;
  readonly drops: number;
  readonly errors: number;
  readonly scrambled: number;
};

// totalQuality は、PID ごとの数を足し合わせる。
export function totalQuality(streams: readonly QualityCounts[]): QualityCounts {
  return streams.reduce<QualityCounts>(
    (sum, stream) => ({
      packets: sum.packets + stream.packets,
      drops: sum.drops + stream.drops,
      errors: sum.errors + stream.errors,
      scrambled: sum.scrambled + stream.scrambled,
    }),
    { packets: 0, drops: 0, errors: 0, scrambled: 0 },
  );
}

// KindBitrates は、PID の種類ごとのビットレート (bps)。
export type KindBitrates = {
  readonly video: number;
  readonly audio: number;
  readonly other: number;
  readonly psi: number;
  readonly unknown: number;
};

// kindBitrates は、PID ごとのパケットの数を種類ごとに足し、seconds 秒で割ったビットレートを返す (#1156)。長さが分からなければ null。
export function kindBitrates(
  streams: readonly { readonly kind: string; readonly packets: number }[],
  seconds: number,
): KindBitrates | null {
  if (seconds <= 0) {
    return null;
  }
  const packets = { video: 0, audio: 0, other: 0, psi: 0, unknown: 0 };
  for (const stream of streams) {
    switch (stream.kind) {
      case 'VIDEO':
        packets.video += stream.packets;
        break;
      case 'AUDIO':
        packets.audio += stream.packets;
        break;
      case 'OTHER':
        packets.other += stream.packets;
        break;
      case 'PSI':
        packets.psi += stream.packets;
        break;
      default:
        packets.unknown += stream.packets;
    }
  }
  const bps = (n: number) => (n * packetSize * 8) / seconds;

  return {
    video: bps(packets.video),
    audio: bps(packets.audio),
    other: bps(packets.other),
    psi: bps(packets.psi),
    unknown: bps(packets.unknown),
  };
}
