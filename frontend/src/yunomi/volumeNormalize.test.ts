import { afterEach, describe, expect, test, vi } from 'vitest';

// FakeNode は、WebAudio のノードの代わり。どこへ繋いだかを覚える。
class FakeNode {
  readonly targets: FakeNode[] = [];
  readonly gain = { value: 1 };
  readonly threshold = { value: 0 };
  readonly knee = { value: 0 };
  readonly ratio = { value: 0 };
  readonly attack = { value: 0 };
  readonly release = { value: 0 };
  readonly delayTime = { value: 0 };
  readonly positionX = { value: 0 };
  readonly positionY = { value: 0 };
  readonly positionZ = { value: 0 };
  channelCount = 2;
  channelCountMode = 'max';
  channelInterpretation = 'speakers';
  constructor(readonly kind: string) {}
  connect(target: FakeNode): FakeNode {
    this.targets.push(target);

    return target;
  }
  disconnect(): void {
    this.targets.length = 0;
  }
}

// FakeAudioContext は、jsdom にない AudioContext の代わり。
class FakeAudioContext {
  state = 'running';
  resumed = 0;
  readonly destination = new FakeNode('destination');
  createGain() {
    return new FakeNode('gain');
  }
  createDynamicsCompressor() {
    return new FakeNode('compressor');
  }
  createDelay() {
    return new FakeNode('delay');
  }
  createMediaElementSource() {
    return new FakeNode('source');
  }
  createChannelSplitter() {
    return new FakeNode('splitter');
  }
  resume() {
    this.resumed += 1;
    this.state = 'running';

    return Promise.resolve();
  }
}

// path は、node から出力までの繋がりを、ノードの種類で並べる。
function path(node: FakeNode): string[] {
  const kinds = [node.kind];
  let current = node;
  while (current.targets.length > 0) {
    current = current.targets[0]!;
    kinds.push(current.kind);
  }

  return kinds;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe('setNormalize', () => {
  test('入れると、コンプレッサーを通し、切ると直に繋ぐ (#499)', async () => {
    vi.stubGlobal('AudioContext', FakeAudioContext);
    const sources: FakeNode[] = [];
    const original = FakeAudioContext.prototype.createMediaElementSource;
    FakeAudioContext.prototype.createMediaElementSource = function () {
      const node = original.call(this);
      sources.push(node);

      return node;
    };
    const { setNormalize, isNormalized } = await import('./volumeBoost');
    // AudioContext は、ユーザーが操作した後でないと動かせない。
    window.dispatchEvent(new Event('pointerdown'));
    const video = document.createElement('video');

    setNormalize(video, true);
    expect(isNormalized(video)).toBe(true);
    expect(path(sources[0]!)).toEqual(['source', 'compressor', 'gain', 'gain', 'destination']);

    setNormalize(video, false);
    expect(isNormalized(video)).toBe(false);
    expect(path(sources[0]!)).toEqual(['source', 'gain', 'destination']);
    FakeAudioContext.prototype.createMediaElementSource = original;
  });

  test('操作の前に入れたら覚えておき、操作した後に入れる', async () => {
    vi.stubGlobal('AudioContext', FakeAudioContext);
    const { setNormalize, isNormalized, resumePendingBoost } = await import('./volumeBoost');
    const video = document.createElement('video');

    setNormalize(video, true);
    expect(isNormalized(video)).toBe(false);

    window.dispatchEvent(new Event('pointerdown'));
    resumePendingBoost(video);
    expect(isNormalized(video)).toBe(true);
  });

  test('入れたときに、止まっている AudioContext を動かす', async () => {
    const contexts: FakeAudioContext[] = [];
    class SuspendedContext extends FakeAudioContext {
      constructor() {
        super();
        this.state = 'suspended';
        contexts.push(this);
      }
    }
    vi.stubGlobal('AudioContext', SuspendedContext);
    const { setNormalize } = await import('./volumeBoost');
    window.dispatchEvent(new Event('pointerdown'));

    setNormalize(document.createElement('video'), true);

    expect(contexts[0]?.resumed).toBe(1);
  });

  test('仮想サラウンドを入れると、音量の補正の手前に挟む (#454)', async () => {
    vi.stubGlobal('AudioContext', FakeAudioContext);
    vi.stubGlobal(
      'PannerNode',
      class extends FakeNode {
        constructor() {
          super('panner');
        }
      },
    );
    const sources: FakeNode[] = [];
    const original = FakeAudioContext.prototype.createMediaElementSource;
    FakeAudioContext.prototype.createMediaElementSource = function () {
      const node = original.call(this);
      sources.push(node);

      return node;
    };
    const { setNormalize, setSurround } = await import('./volumeBoost');
    window.dispatchEvent(new Event('pointerdown'));
    const video = document.createElement('video');

    setSurround(video, true);
    // L のチャンネルは、倍率、HRTF の PannerNode、2ch に混ぜる GainNode を通って、音量の GainNode へ届く。
    expect(path(sources[0]!)).toEqual([
      'source',
      'splitter',
      'gain',
      'panner',
      'gain',
      'gain',
      'destination',
    ]);

    setNormalize(video, true);
    expect(path(sources[0]!)).toEqual([
      'source',
      'splitter',
      'gain',
      'panner',
      'gain',
      'compressor',
      'gain',
      'gain',
      'destination',
    ]);

    setSurround(video, false);
    expect(path(sources[0]!)).toEqual(['source', 'compressor', 'gain', 'gain', 'destination']);
    FakeAudioContext.prototype.createMediaElementSource = original;
  });
});

describe('setAudioDelay', () => {
  // RecordingContext は、作った元の音と DelayNode を覚える AudioContext。
  const recordingContext = () => {
    const sources: FakeNode[] = [];
    const delays: FakeNode[] = [];
    class RecordingContext extends FakeAudioContext {
      override createMediaElementSource() {
        const node = super.createMediaElementSource();
        sources.push(node);

        return node;
      }
      override createDelay() {
        const node = super.createDelay();
        delays.push(node);

        return node;
      }
    }

    return { RecordingContext, sources, delays };
  };

  test('遅らせると、最初の段に DelayNode を挟んで長さを当て、0 にすると外す (#818)', async () => {
    const { RecordingContext, sources, delays } = recordingContext();
    vi.stubGlobal('AudioContext', RecordingContext);
    const { setAudioDelay, setNormalize } = await import('./volumeBoost');
    window.dispatchEvent(new Event('pointerdown'));
    const video = document.createElement('video');

    setNormalize(video, true);
    setAudioDelay(video, 0.12);
    expect(path(sources[0]!)).toEqual([
      'source',
      'delay',
      'compressor',
      'gain',
      'gain',
      'destination',
    ]);
    expect(delays[0]?.delayTime.value).toBe(0.12);

    // 長さだけを変えたときも当てる。
    setAudioDelay(video, 0.2);
    expect(delays[0]?.delayTime.value).toBe(0.2);

    setAudioDelay(video, 0);
    expect(path(sources[0]!)).toEqual(['source', 'compressor', 'gain', 'gain', 'destination']);
  });

  test('操作の前に遅らせたら覚えておき、操作した後に、覚えた長さで入れる', async () => {
    const { RecordingContext, delays } = recordingContext();
    vi.stubGlobal('AudioContext', RecordingContext);
    const { setAudioDelay, resumePendingBoost } = await import('./volumeBoost');
    const video = document.createElement('video');

    setAudioDelay(video, 0.1);
    expect(delays).toHaveLength(0);

    window.dispatchEvent(new Event('pointerdown'));
    resumePendingBoost(video);
    expect(delays[0]?.delayTime.value).toBe(0.1);
  });
});
