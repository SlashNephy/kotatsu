import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { beforeAll, describe, expect, test } from 'vitest';

import {
  clockHz,
  indexStream,
  packetSize,
  type StreamIndex,
  StreamLooper,
} from '../../demo/stream-loop.js';

// period は、デモの映像の長さ (映像の 1920 フレーム、90 kHz の刻み)。scripts/build-demo-stream.sh で作る長さ。
const period = 1920 * 3003;
const videoPID = 0x100;
const audioPID = 0x101;

type Packet = {
  pid: number;
  unitStart: boolean;
  randomAccess: boolean;
  cc: number;
  payload: Uint8Array | null;
};

// parse は、TS をパケットに分ける。
function parse(bytes: Uint8Array): Packet[] {
  const packets: Packet[] = [];
  for (let offset = 0; offset < bytes.length; offset += packetSize) {
    const control = (bytes[offset + 3]! >> 4) & 0x03;
    let start = 4;
    let randomAccess = false;
    if ((control & 0x02) !== 0) {
      const length = bytes[offset + 4]!;
      randomAccess = length > 0 && (bytes[offset + 5]! & 0x40) !== 0;
      start = 5 + length;
    }
    packets.push({
      pid: ((bytes[offset + 1]! & 0x1f) << 8) | bytes[offset + 2]!,
      unitStart: (bytes[offset + 1]! & 0x40) !== 0,
      randomAccess,
      cc: bytes[offset + 3]! & 0x0f,
      payload: (control & 0x01) === 0 ? null : bytes.subarray(offset + start, offset + packetSize),
    });
  }

  return packets;
}

// pts は、PES の頭のパケットから PTS を読む。
function pts(payload: Uint8Array): number {
  return (
    ((payload[9]! >> 1) & 0x07) * 2 ** 30 +
    payload[10]! * 2 ** 22 +
    (payload[11]! >> 1) * 2 ** 15 +
    payload[12]! * 2 ** 7 +
    (payload[13]! >> 1)
  );
}

// readAll は、流し終えるまで読み、つなげて返す。limit は、読みすぎて止まらないときの上限のパケット数。
function readAll(looper: StreamLooper, limit: number): Uint8Array {
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (let chunk = looper.read(1000); chunk !== null; chunk = looper.read(1000)) {
    chunks.push(chunk);
    total += chunk.length;
    if (total > limit * packetSize) {
      throw new Error('the looper does not stop');
    }
  }
  const out = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
  }

  return out;
}

// timestamps は、PID の PES の PTS を、届く順に返す。
function timestamps(packets: Packet[], pid: number): number[] {
  return packets
    .filter((packet) => packet.pid === pid && packet.unitStart && packet.payload !== null)
    .map((packet) => pts(packet.payload!));
}

describe('StreamLooper', () => {
  let index: StreamIndex;
  let source: Packet[];
  let firstVideoPTS: number;
  beforeAll(() => {
    // テストは frontend をルートに動かす。jsdom の環境では import.meta.url がファイルの URL にならない。
    const bytes = new Uint8Array(readFileSync(join(process.cwd(), 'demo/stream.m2ts')));
    index = indexStream(bytes, period);
    source = parse(bytes);
    firstVideoPTS = timestamps(source, videoPID)[0]!;
  });

  test('continues the timestamps and the continuity counters across the loops', () => {
    const out = readAll(new StreamLooper(index, { start: 0, end: 2.5 * period }), index.count * 3);
    const packets = parse(out);

    // 繰り返すたびに、元の映像の PTS を period ずつ進めた並びになる。2 回を越えて流し、3 回目に入っている。
    for (const pid of [videoPID, audioPID]) {
      const original = timestamps(source, pid);
      const looped = timestamps(packets, pid);
      const expected = [0, 1, 2].flatMap((loop) => original.map((value) => value + loop * period));
      expect(looped.length).toBeGreaterThan(original.length * 2);
      expect(looped).toEqual(expected.slice(0, looped.length));
    }
    // 映像は、つなぎ目でも 1 フレームずつ進む。
    const video = timestamps(packets, videoPID).sort((a, b) => a - b);
    expect(new Set(video.slice(1).map((value, i) => value - video[i]!))).toEqual(new Set([3003]));
    const last = new Map<number, number>();
    for (const packet of packets.filter((candidate) => candidate.payload !== null)) {
      const previous = last.get(packet.pid);
      if (previous !== undefined) {
        expect((previous + 1) & 0x0f).toBe(packet.cc);
      }
      last.set(packet.pid, packet.cc);
    }
  });

  test('starts from the PAT before the key frame at or before the requested position', () => {
    const start = 100 * clockHz;
    const looper = new StreamLooper(index, { start, end: start + 5 * clockHz });
    const packets = parse(readAll(looper, index.count));

    expect(looper.started).toBeLessThanOrEqual(start);
    expect(looper.started).toBeGreaterThan(start - 3 * clockHz);
    expect(packets[0]!.pid).toBe(0);
    const firstVideo = packets.find((packet) => packet.pid === videoPID && packet.unitStart)!;
    expect(firstVideo.randomAccess).toBe(true);
    expect(pts(firstVideo.payload!)).toBe(firstVideoPTS + looper.started);
  });

  test('stops before the end at a point where no frame is cut', () => {
    const end = 30 * clockHz;
    const packets = parse(readAll(new StreamLooper(index, { start: 0, end }), index.count));

    // 映像は、終わりを越えず、終わりの手前の I フレームの間隔 (約 3 秒) の内まで、1 フレームずつ欠けずに届く。
    const video = timestamps(packets, videoPID).sort((a, b) => a - b);
    expect(video.at(-1)! - firstVideoPTS).toBeLessThan(end);
    expect(video.at(-1)! - firstVideoPTS).toBeGreaterThan(end - 3 * clockHz);
    expect(new Set(video.slice(1).map((value, i) => value - video[i]!))).toEqual(new Set([3003]));
    // 音声の PES は長さを書くので、最後の PES まで、書いた長さだけ届いている。
    let expected = 0;
    let received = 0;
    for (const packet of packets.filter((candidate) => candidate.pid === audioPID)) {
      if (packet.unitStart) {
        expect(received).toBe(expected);
        expected = ((packet.payload![4]! << 8) | packet.payload![5]!) + 6;
        received = 0;
      }
      received += packet.payload?.length ?? 0;
    }
    expect(received).toBe(expected);
  });

  test('sends at least the key frame interval when starting just before the end', () => {
    const end = 100 * clockHz;
    const packets = parse(
      readAll(new StreamLooper(index, { start: end - clockHz / 2, end }), index.count),
    );

    expect(timestamps(packets, videoPID).length).toBeGreaterThan(30);
  });

  test('keeps streaming without an end', () => {
    const looper = new StreamLooper(index);
    for (let read = 0; read < (index.count * 3) / 1000; read++) {
      expect(looper.read(1000)).not.toBeNull();
    }

    expect(looper.position).toBeGreaterThan(2 * period);
  });
});
