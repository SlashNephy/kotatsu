import { afterEach, describe, expect, test, vi } from 'vitest';

import { createCaptions } from './captions';

// fed は、偽の aribb24.js の feeder に渡ったもの。種類 (字幕か文字スーパーか) ごとに、渡った順に並べる。
const fed: { type: string; kind: string; pts: number }[] = [];
// texts は、偽の aribb24.js の Controller に付けた、字幕の文を取り出す描画器。
const texts: { render: () => void; clear: () => void }[] = [];
vi.mock('aribb24.js', () => {
  return {
    Controller: class {
      attachFeeder() {}
      attachRenderer(renderer: { render?: () => void; getText?: () => string }) {
        if (renderer.getText !== undefined) {
          texts.push(renderer as { render: () => void; clear: () => void });
        }
      }
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
    TextRenderer: class {
      render() {}
      clear() {}
      getText() {
        return '字幕の文';
      }
      destroy() {}
    },
    CanvasMainThreadRenderer: class {
      private readonly canvas = document.createElement('canvas');
      getPresentationCanvas() {
        return this.canvas;
      }
      clear() {}
      destroy() {}
    },
  };
});

afterEach(() => {
  fed.length = 0;
  texts.length = 0;
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

  test('字幕の層にだけ、文を取り出す描画器を付け、字幕を描くたびに文を渡し、消えたら空の文を渡す', async () => {
    const onText = vi.fn();
    const captions = createCaptions(
      document.createElement('video'),
      document.createElement('div'),
      onText,
    );
    await vi.waitFor(() => expect(texts).toHaveLength(1));
    texts[0]!.render();
    expect(onText).toHaveBeenLastCalledWith('字幕の文');

    // 字幕が消えたら、空の文を渡す。キャプチャに、消えた字幕を書き込まないようにする (#458)。
    texts[0]!.clear();
    expect(onText).toHaveBeenLastCalledWith('');

    // 読み直すときも、今出ている字幕を消したことを渡す。
    texts[0]!.render();
    captions.reset();
    expect(onText).toHaveBeenLastCalledWith('');
  });

  test('文を受け取らないなら、文を取り出す描画器を付けない', async () => {
    const captions = createCaptions(document.createElement('video'), document.createElement('div'));
    captions.feedB24(new ArrayBuffer(1), 1);
    await vi.waitFor(() => expect(fed).toHaveLength(2));

    expect(texts).toHaveLength(0);
  });
});
