import { type Gap, recordingPosition } from '../yunomi/position';

// 端末に保存した録画 (設計書 7.8) を、指定した位置から読ませる。この端末でデコードする経路は、映像の要素が
// シークできないので、ファイルの途中から切り出して読み直す (#2166)。どこから切り出すかは、TS の PCR を
// 二分探索して決める。ファイルの大きさに比例させるだけでは、ビットレートの揺れで数秒ずれるため。

// packetSize は、TS のパケットの大きさ。
const packetSize = 188;
// scanPackets は、PCR を探すのに、1 か所から読むパケットの数。放送の PCR は 0.1 秒以内ごとに入るので、
// 24 Mbps でも 0.1 秒分 (約 1600 パケット) を読めば見つかる。
const scanPackets = 2000;
// pcrWrap は、33 ビットの PCR の base が一周する秒数。
const pcrWrap = 2 ** 33 / 90_000;
// leadSeconds は、求めた位置より手前から読み始める秒数。I フレームを待つ間に、求めた位置を過ぎないようにする。
const leadSeconds = 1;

// SavedRecording は、切り出す録画のファイルと、位置を求めるのに使う録り始めと途切れた区間。
export type SavedRecording = {
  video: Blob;
  recordedFrom: string;
  gaps: readonly Gap[];
};

// StreamFrom は、切り出した映像と、その先頭の再生の位置 (秒)。
export type StreamFrom = { blob: Blob; at: number };

// firstPCR は、offset から先で最初に見つかった PCR の、パケットの位置と時刻 (秒) を返す。見つからなければ null。
export async function firstPCR(
  video: Blob,
  offset: number,
): Promise<{ offset: number; seconds: number } | null> {
  const start = Math.max(0, Math.floor(offset / packetSize) * packetSize);
  const bytes = new Uint8Array(
    await video.slice(start, start + packetSize * (scanPackets + 3)).arrayBuffer(),
  );
  // 切り出した先頭がパケットの境目とは限らないので、3 つ続けて同期バイトが並ぶ所を境目とみなす。
  let sync = 0;
  while (
    sync < packetSize &&
    !(
      bytes[sync] === 0x47 &&
      bytes[sync + packetSize] === 0x47 &&
      bytes[sync + packetSize * 2] === 0x47
    )
  ) {
    sync += 1;
  }
  if (sync === packetSize) {
    return null;
  }
  for (let p = sync; p + packetSize <= bytes.length; p += packetSize) {
    if (bytes[p] !== 0x47) {
      continue;
    }
    const adaptation = ((bytes[p + 3] ?? 0) >> 4) & 0b11;
    // アダプテーションフィールドがあり、長さが PCR に届き、PCR のフラグが立っているパケット。
    if (
      (adaptation & 0b10) !== 0 &&
      (bytes[p + 4] ?? 0) >= 7 &&
      ((bytes[p + 5] ?? 0) & 0x10) !== 0
    ) {
      const b = bytes.subarray(p + 6, p + 11);
      const base =
        (b[0] ?? 0) * 2 ** 25 +
        (b[1] ?? 0) * 2 ** 17 +
        (b[2] ?? 0) * 2 ** 9 +
        (b[3] ?? 0) * 2 +
        ((b[4] ?? 0) >> 7);

      return { offset: start + p, seconds: base / 90_000 };
    }
  }

  return null;
}

// streamFrom は、録画 recording を、再生の位置 seconds の少し手前から読ませる映像を切り出す。
// PCR は録り始めからの経過で、途切れた区間の分も進むので、録画の時間軸の時刻に直してから、途切れた分を差し引く。
export async function streamFrom(recording: SavedRecording, seconds: number): Promise<StreamFrom> {
  const { video } = recording;
  const head = await firstPCR(video, 0);
  if (head === null || seconds <= leadSeconds) {
    return { blob: video, at: 0 };
  }
  const startedAt = Date.parse(recording.recordedFrom);
  const positionOf = (pcr: number) => {
    const elapsed = (pcr - head.seconds + pcrWrap) % pcrWrap;

    return recordingPosition(recording.recordedFrom, recording.gaps, startedAt + elapsed * 1000);
  };
  const target = seconds - leadSeconds;
  // low は、位置が target 以下と分かっているパケットの位置。high は、target を超えると分かっている位置。
  let low = { offset: head.offset, at: 0 };
  let high = video.size;
  while (high - low.offset > packetSize * scanPackets) {
    const middle = Math.floor((low.offset + high) / 2 / packetSize) * packetSize;
    const found = await firstPCR(video, middle);
    if (found === null || found.offset >= high) {
      high = middle;
      continue;
    }
    const at = positionOf(found.seconds);
    if (at <= target) {
      low = { offset: found.offset, at };
    } else {
      high = middle;
    }
  }

  return { blob: video.slice(low.offset), at: low.at };
}
