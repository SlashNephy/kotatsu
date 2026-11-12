// demo/stream-loop.js の型。Service Worker (demo/stream-sw.js) はビルドせずにそのまま配るので、実装は JavaScript で書き、
// テスト (src/demo/streamLoop.test.ts) から読むための型だけをここに書く。

export const packetSize: number;
export const clockHz: number;

export type StreamIndex = {
  readonly bytes: Uint8Array;
  readonly count: number;
  readonly period: number;
  readonly keys: ReadonlyArray<{ readonly packet: number; readonly position: number }>;
  readonly time: Float64Array;
};

export function indexStream(bytes: Uint8Array, period: number): StreamIndex;

export class StreamLooper {
  constructor(index: StreamIndex, options?: { start?: number; end?: number | null });
  readonly started: number;
  readonly position: number;
  readonly done: boolean;
  read(packets: number): Uint8Array | null;
}
