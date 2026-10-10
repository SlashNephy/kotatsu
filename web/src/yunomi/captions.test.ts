import { afterEach, describe, expect, test, vi } from 'vitest';

import { createCaptions } from './captions';

// fed は、偽の aribb24.js の feeder に渡ったもの。種類 (字幕か文字スーパーか) ごとに、渡った順に並べる。
const fed: { type: string; kind: string; pts: number }[] = [];
vi.mock('aribb24.js', () => {
  return {
    Controller: class {
      attachFeeder() {}
      attachRenderer() {}
      attachMedia() {}
      detachMedia() {}
      detachFeeder() {}
      detachRenderer() {}
      show() {}
      hide() {}
    },
    MPEGTSFeeder: class {
      private readonly type: string;
      constructor(option: { receive: { type: string } }) {
        this.type = option.receive.type;
      }
      feedB24(_data: ArrayBuffer, pts: number) {
        fed.push({ type: this.type, kind: 'b24', pts });
      }
      feedID3(_data: Uint8Array, pts: number) {
        fed.push({ type: this.type, kind: 'id3', pts });
      }
      clear() {}
    },
    CanvasMainThreadRenderer: class {
      clear() {}
      destroy() {}
    },
  };
});

afterEach(() => {
  fed.length = 0;
  vi.resetModules();
});

describe('createCaptions', () => {
  test('aribb24.js を読み終える前に届いた字幕も、届いた順に、字幕と文字スーパーの両方に渡す', async () => {
    const captions = createCaptions(document.createElement('video'), document.createElement('div'));
    captions.feedB24(new ArrayBuffer(1), 1);
    captions.feedID3(new Uint8Array(1), 2);
    await vi.waitFor(() => expect(fed).toHaveLength(4));
    captions.feedB24(new ArrayBuffer(1), 3);

    expect(fed).toEqual([
      { type: 'Caption', kind: 'b24', pts: 1 },
      { type: 'Superimpose', kind: 'b24', pts: 1 },
      { type: 'Caption', kind: 'id3', pts: 2 },
      { type: 'Superimpose', kind: 'id3', pts: 2 },
      { type: 'Caption', kind: 'b24', pts: 3 },
      { type: 'Superimpose', kind: 'b24', pts: 3 },
    ]);
  });

  test('読み終える前に捨てた字幕は渡さない', async () => {
    const captions = createCaptions(document.createElement('video'), document.createElement('div'));
    captions.feedB24(new ArrayBuffer(1), 1);
    captions.reset();
    captions.feedB24(new ArrayBuffer(1), 2);
    await vi.waitFor(() => expect(fed).toHaveLength(2));

    expect(fed.map((item) => item.pts)).toEqual([2, 2]);
  });
});
