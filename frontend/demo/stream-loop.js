// 静的なデモ (#832) の映像 (demo/stream.m2ts) を、繰り返して 1 本の続いた放送のように流すための、TS の書き換え。
// demo/stream-sw.js が使い、src/demo/streamLoop.test.ts で確かめる。
//
// 映像は、映像と音声のフレームがちょうど収まる長さ (period) で作ってある (scripts/build-demo-stream.sh)。
// 繰り返すたびに、PCR、PTS、DTS を period ずつ進め、連続性指標 (continuity_counter) も前の繰り返しの続きにする。
// プレイヤーからは、時刻が戻らず、パケットも欠けない、途切れずに続く TS に見える。

export const packetSize = 188;
// clockHz は、PCR の基底、PTS、DTS の刻み。
export const clockHz = 90000;
// wrap は、PCR の基底、PTS、DTS が一周する値 (33 ビット)。
const wrap = 2 ** 33;

// readTimestamp は、PES の PTS か DTS (5 バイト) を読む。
function readTimestamp(bytes, at) {
  return (
    ((bytes[at] >> 1) & 0x07) * 2 ** 30 +
    bytes[at + 1] * 2 ** 22 +
    (bytes[at + 2] >> 1) * 2 ** 15 +
    bytes[at + 3] * 2 ** 7 +
    (bytes[at + 4] >> 1)
  );
}

// writeTimestamp は、PES の PTS か DTS を書く。頭の 4 ビット (PTS か DTS かの印) と、間の印のビットは残す。
function writeTimestamp(bytes, at, value) {
  bytes[at] = (bytes[at] & 0xf1) | ((Math.floor(value / 2 ** 30) & 0x07) << 1);
  bytes[at + 1] = Math.floor(value / 2 ** 22) & 0xff;
  bytes[at + 2] = ((Math.floor(value / 2 ** 15) & 0x7f) << 1) | 0x01;
  bytes[at + 3] = Math.floor(value / 2 ** 7) & 0xff;
  bytes[at + 4] = ((value & 0x7f) << 1) | 0x01;
}

// readPCR は、アダプテーションフィールドの PCR の基底 (33 ビット) を読む。拡張 (27 MHz の端数) は使わない。
function readPCR(bytes, at) {
  return (
    bytes[at] * 2 ** 25 +
    bytes[at + 1] * 2 ** 17 +
    bytes[at + 2] * 2 ** 9 +
    bytes[at + 3] * 2 +
    (bytes[at + 4] >> 7)
  );
}

// writePCR は、PCR の基底を書く。続く予約のビットと拡張は残す。
function writePCR(bytes, at, value) {
  bytes[at] = Math.floor(value / 2 ** 25) & 0xff;
  bytes[at + 1] = Math.floor(value / 2 ** 17) & 0xff;
  bytes[at + 2] = Math.floor(value / 2 ** 9) & 0xff;
  bytes[at + 3] = Math.floor(value / 2) & 0xff;
  bytes[at + 4] = (bytes[at + 4] & 0x7f) | ((value & 0x01) << 7);
}

// indexStream は、TS (bytes) を読み、繰り返して流すのに要る位置を集める。period は、繰り返しの長さ (90 kHz の刻み)。
// 返す time は、パケットごとの、映像の頭からの位置 (90 kHz の刻み)。PCR から、映像の頭の PTS との差を引いて数える。
export function indexStream(bytes, period) {
  if (bytes.length % packetSize !== 0) {
    throw new Error('the stream is not a sequence of 188-byte packets');
  }
  const count = bytes.length / packetSize;
  const pids = new Uint16Array(count);
  // pcrAt、ptsAt、dtsAt は、パケットの中の位置。なければ 0 (TS のヘッダーの中なので、ありえない位置)。
  const pcrAt = new Uint8Array(count);
  const ptsAt = new Uint8Array(count);
  const dtsAt = new Uint8Array(count);
  const pcrs = new Float64Array(count).fill(Number.NaN);
  // increments は、PID ごとの、連続性指標が進むパケット (ペイロードのあるもの) の数。
  const increments = new Map();
  // keys は、映像の I フレーム (ランダムアクセスの印のある、PES の頭) のパケットの位置と PTS。
  const keys = [];
  // boundary は、そのパケットの手前で切っても、どのフレームも欠けないかどうか。映像の I フレームの PES の頭で、
  // 長さの決まった PES (音声と字幕) を送り終えているところ。GOP は閉じてあるので、I フレームの手前までの映像は、
  // そこまでで映し終わる。映像の PES は長さを書かない (0) ので、次の PES の頭か、TS の終わりで終わる。
  const boundary = new Uint8Array(count);
  // remaining は、PID ごとの、長さの決まった PES の、まだ送っていないバイト数。
  const remaining = new Map();
  let videoPID = null;
  let firstVideoPTS = null;
  let pcrAtFirstVideo = null;
  let lastPCR = Number.NaN;
  let lastPAT = 0;
  for (let index = 0; index < count; index++) {
    const offset = index * packetSize;
    if (bytes[offset] !== 0x47) {
      throw new Error(`packet ${index} has no sync byte`);
    }
    const pid = ((bytes[offset + 1] & 0x1f) << 8) | bytes[offset + 2];
    const unitStart = (bytes[offset + 1] & 0x40) !== 0;
    const control = (bytes[offset + 3] >> 4) & 0x03;
    pids[index] = pid;
    if (pid === 0) {
      lastPAT = index;
    }
    let payload = 4;
    let randomAccess = false;
    if ((control & 0x02) !== 0) {
      const length = bytes[offset + 4];
      if (length > 0) {
        const flags = bytes[offset + 5];
        randomAccess = (flags & 0x40) !== 0;
        if ((flags & 0x10) !== 0) {
          pcrAt[index] = 6;
          lastPCR = readPCR(bytes, offset + 6);
        }
      }
      payload = 5 + length;
    }
    pcrs[index] = lastPCR;
    if ((control & 0x01) === 0) {
      continue;
    }
    increments.set(pid, (increments.get(pid) ?? 0) + 1);
    const at = offset + payload;
    if (!unitStart && remaining.has(pid)) {
      remaining.set(pid, remaining.get(pid) - (packetSize - payload));
    }
    // PES の頭なら、PTS と DTS の位置を覚える。PSI (PAT や PMT) は 00 00 01 で始まらない。
    if (
      !unitStart ||
      payload + 14 > packetSize ||
      bytes[at] !== 0 ||
      bytes[at + 1] !== 0 ||
      bytes[at + 2] !== 1 ||
      (bytes[at + 6] & 0xc0) !== 0x80
    ) {
      continue;
    }
    const streamID = bytes[at + 3];
    const flags = bytes[at + 7] >> 6;
    const length = (bytes[at + 4] << 8) | bytes[at + 5];
    if (length > 0) {
      remaining.set(pid, length + 6 - (packetSize - payload));
    }
    if (flags >= 2) {
      ptsAt[index] = payload + 9;
    }
    if (flags === 3) {
      dtsAt[index] = payload + 14;
    }
    if (ptsAt[index] === 0 || (streamID & 0xf0) !== 0xe0) {
      continue;
    }
    // 映像の PES (stream_id が 0xe0 から 0xef)。
    videoPID ??= pid;
    if (pid !== videoPID) {
      continue;
    }
    if (randomAccess && [...remaining.values()].every((left) => left <= 0)) {
      boundary[index] = 1;
    }
    const pts = readTimestamp(bytes, offset + ptsAt[index]);
    if (firstVideoPTS === null) {
      firstVideoPTS = pts;
      pcrAtFirstVideo = lastPCR;
    }
    if (randomAccess) {
      // 流し始めるのは、その I フレームの手前の PAT から。プレイヤーが PAT と PMT を先に読めるようにする。
      keys.push({ packet: lastPAT, position: pts - firstVideoPTS });
    }
  }
  if (firstVideoPTS === null || Number.isNaN(pcrAtFirstVideo)) {
    throw new Error('the stream has no video PES with PCR');
  }
  // PCR は、届く時刻なので、映される時刻 (PTS) より先を進む。映像の頭で揃え、パケットの位置を映される時刻で数える。
  const lead = firstVideoPTS - pcrAtFirstVideo;
  const firstPCR = pcrs.find((pcr) => !Number.isNaN(pcr));
  const time = new Float64Array(count);
  for (let index = 0; index < count; index++) {
    const pcr = Number.isNaN(pcrs[index]) ? firstPCR : pcrs[index];
    time[index] = pcr + lead - firstVideoPTS;
  }
  // nextBoundary は、切れ目のパケットごとの、次の切れ目の位置 (90 kHz の刻み)。最後の切れ目の次は、次の繰り返しの最初の切れ目。
  const nextBoundary = new Float64Array(count);
  let next = Number.NaN;
  for (let index = count - 1; index >= 0; index--) {
    if (boundary[index] === 1) {
      nextBoundary[index] = next;
      next = time[index];
    }
  }
  for (let index = count - 1; index >= 0; index--) {
    if (boundary[index] === 1) {
      nextBoundary[index] = period + next;
      break;
    }
  }

  return {
    bytes,
    count,
    period,
    pids,
    pcrAt,
    ptsAt,
    dtsAt,
    increments,
    keys,
    boundary,
    nextBoundary,
    time,
  };
}

// StreamLooper は、繰り返した TS を、頭から順に切り出す。
// start は、流し始める位置 (90 kHz の刻み、映像を繰り返した続きの中の位置)。その手前の I フレームから流す。
// end は、流し終える位置。null なら終わらない。
export class StreamLooper {
  constructor(index, { start = 0, end = null } = {}) {
    this.index = index;
    this.end = end;
    const loop = Math.floor(start / index.period);
    const phase = start - loop * index.period;
    const key = index.keys.findLast((candidate) => candidate.position <= phase) ?? index.keys[0];
    this.loop = loop;
    this.packet = key.packet;
    // started は、実際に流し始める位置 (90 kHz の刻み)。
    this.started = loop * index.period + key.position;
    // sentFrame は、切れ目を 1 つでも流したかどうか。終わりの間際から流し始めても、I フレームからの 1 区切りは流す。
    this.sentFrame = false;
  }

  // position は、次に切り出すパケットの位置 (90 kHz の刻み、映像を繰り返した続きの中の位置)。
  get position() {
    return this.loop * this.index.period + this.index.time[this.packet];
  }

  // done は、流し終えたかどうか。フレームの欠けない切れ目のうち、次の切れ目まで流すと流し終える位置を越えるところで止める。
  // 録画の終わりを越えて流さず、終わりの手前の I フレームの間隔 (約 3 秒) の内で終える。
  get done() {
    const { boundary, nextBoundary, period } = this.index;

    return (
      this.end !== null &&
      this.sentFrame &&
      boundary[this.packet] === 1 &&
      this.loop * period + nextBoundary[this.packet] > this.end
    );
  }

  // read は、次の packets 個までのパケットを、時刻と連続性指標を書き換えて返す。流し終えていたら null を返す。
  read(packets) {
    const { bytes, count, period, pids, pcrAt, ptsAt, dtsAt, increments } = this.index;
    const out = new Uint8Array(packets * packetSize);
    let written = 0;
    while (written < packets && !this.done) {
      const source = this.packet;
      const offset = written * packetSize;
      out.set(bytes.subarray(source * packetSize, (source + 1) * packetSize), offset);
      if (this.loop > 0) {
        const shift = (this.loop * period) % wrap;
        const cc = out[offset + 3] & 0x0f;
        out[offset + 3] =
          (out[offset + 3] & 0xf0) |
          ((cc + this.loop * (increments.get(pids[source]) ?? 0)) & 0x0f);
        if (pcrAt[source] !== 0) {
          const at = offset + pcrAt[source];
          writePCR(out, at, (readPCR(out, at) + shift) % wrap);
        }
        if (ptsAt[source] !== 0) {
          const at = offset + ptsAt[source];
          writeTimestamp(out, at, (readTimestamp(out, at) + shift) % wrap);
        }
        if (dtsAt[source] !== 0) {
          const at = offset + dtsAt[source];
          writeTimestamp(out, at, (readTimestamp(out, at) + shift) % wrap);
        }
      }
      if (this.index.boundary[source] === 1) {
        this.sentFrame = true;
      }
      written++;
      this.packet++;
      if (this.packet === count) {
        this.packet = 0;
        this.loop++;
      }
    }
    if (written === 0) {
      return null;
    }

    return out.subarray(0, written * packetSize);
  }
}
