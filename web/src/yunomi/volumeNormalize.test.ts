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
  readonly destination = new FakeNode('destination');
  createGain() {
    return new FakeNode('gain');
  }
  createDynamicsCompressor() {
    return new FakeNode('compressor');
  }
  createMediaElementSource() {
    return new FakeNode('source');
  }
  resume() {
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
});
