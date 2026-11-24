import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, test } from 'vitest';

import { clockHz, indexStream, packetSize } from '../../demo/stream-loop.js';
import { benchmarkClips, clipPeriod, loopClip } from './benchmarkStream';

// readClip は、配信している試す映像を読む。テストは frontend をルートに動かす。
function readClip(path: string): Uint8Array {
  return new Uint8Array(readFileSync(join(process.cwd(), 'public', path)));
}

describe('loopClip', () => {
  test.each(Object.entries(benchmarkClips))(
    '%s を、映像の長さ (clipPeriod) で繰り返せる',
    (_name, path) => {
      const bytes = readClip(path);
      const looped = loopClip(bytes, 5);
      // 繰り返した TS も、時刻を読み直せる 188 バイトのパケットの並び。
      const index = indexStream(looped, clipPeriod * 5);
      const seconds = (index.count * clipPeriod) / clockHz / (bytes.length / packetSize);

      expect(looped.length % packetSize).toBe(0);
      // I フレームの切れ目で止めるので、5 秒より少し短いところまで流す。
      expect(seconds).toBeGreaterThan(4);
      expect(seconds).toBeLessThanOrEqual(5);
      // 繰り返した各回の先頭の I フレームが、映像の長さずつ進む。
      const loopStarts = index.keys
        .map((key) => key.position)
        .filter((position) => position % clipPeriod === 0);
      expect(loopStarts.slice(0, 4)).toEqual([0, 1, 2, 3].map((loop) => loop * clipPeriod));
    },
  );
});
