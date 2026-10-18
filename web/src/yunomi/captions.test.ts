import { afterEach, describe, expect, test, vi } from 'vitest';

import { createCaptions, rendererColor } from './captions';

// fed は、偽の aribb24.js の feeder に渡ったもの。種類 (字幕か文字スーパーか) ごとに、渡った順に並べる。
const fed: { type: string; kind: string; pts: number }[] = [];
// texts は、偽の aribb24.js の Controller に付けた、字幕の文を取り出す描画器。
const texts: { render: () => void; clear: () => void }[] = [];
// shown は、偽の aribb24.js の Controller ごとの、最後に出したか隠したか。種類 (字幕か文字スーパーか) で引く。
const shown = new Map<string, boolean>();
// colors は、偽の描画器の色の設定。種類で引けないので、作った順に並べる (字幕、文字スーパーの順)。
const colors: { stroke: string | null; background: string | null }[] = [];
vi.mock('aribb24.js', () => {
  return {
    Controller: class {
      private type = '';
      attachFeeder(feeder: { type: string }) {
        this.type = feeder.type;
      }
      attachRenderer(renderer: { render?: () => void; getText?: () => string }) {
        if (renderer.getText !== undefined) {
          texts.push(renderer as { render: () => void; clear: () => void });
        }
      }
      attachMedia() {}
      detachMedia() {}
      detachFeeder() {}
      detachRenderer() {}
      show() {
        shown.set(this.type, true);
      }
      hide() {
        shown.set(this.type, false);
      }
    },
    MPEGTSFeeder: class {
      readonly type: string;
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
      // option は、aribb24.js の描画器が、描くたびに読む設定。
      readonly option = { color: { stroke: null, background: null } };
      constructor() {
        colors.push(this.option.color);
      }
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
  shown.clear();
  colors.length = 0;
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

  test('文字スーパーは、字幕とは別に隠せる。字幕を隠せば、どちらも隠す (#491)', async () => {
    const captions = createCaptions(document.createElement('video'), document.createElement('div'));
    await vi.waitFor(() => expect(shown.size).toBe(2));

    captions.setSuperimposeVisible(false);
    expect(Object.fromEntries(shown)).toEqual({ Caption: true, Superimpose: false });

    captions.setSuperimposeVisible(true);
    captions.setVisible(false);
    expect(Object.fromEntries(shown)).toEqual({ Caption: false, Superimpose: false });
  });

  test('見た目を変えると、どちらの層の描画器の色の設定も変える (#491)', async () => {
    const captions = createCaptions(document.createElement('video'), document.createElement('div'));
    await vi.waitFor(() => expect(colors).toHaveLength(2));

    captions.setStyle({ stroke: true, backgroundOpacity: 0.5 });

    expect(colors).toEqual([
      { stroke: 'black', background: 'rgba(0, 0, 0, 0.5)' },
      { stroke: 'black', background: 'rgba(0, 0, 0, 0.5)' },
    ]);
  });
});

describe('rendererColor', () => {
  test.each([
    {
      name: '放送の指定どおり',
      style: { stroke: false, backgroundOpacity: null },
      want: { stroke: null, background: null },
    },
    {
      name: '縁取り、背景は透明',
      style: { stroke: true, backgroundOpacity: 0 },
      want: { stroke: 'black', background: 'rgba(0, 0, 0, 0)' },
    },
  ])('$name', ({ style, want }) => {
    expect(rendererColor(style)).toEqual(want);
  });
});
