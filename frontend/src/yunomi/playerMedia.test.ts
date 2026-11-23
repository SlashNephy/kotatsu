import { afterEach, describe, expect, test, vi } from 'vitest';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.resetModules();
});

describe('mediaOf', () => {
  test('同じ <video> には同じものを、別の <video> には別のものを返す', async () => {
    const { mediaOf } = await import('./playerMedia');
    const video = document.createElement('video');
    const other = document.createElement('video');

    expect(mediaOf(video)).toBe(mediaOf(video));
    expect(mediaOf(other)).not.toBe(mediaOf(video));
  });
});

describe('コマごとの呼び返しと、コマの数', () => {
  // 部品は、メソッドの有無で requestAnimationFrame などに切り替える。対応していない <video> でも
  // メソッドがあると、切り替えずに呼んで、例外になる。
  test('<video> が持たなければ、持たない', async () => {
    const { mediaOf } = await import('./playerMedia');
    const video = document.createElement('video');
    Object.defineProperty(video, 'requestVideoFrameCallback', { value: undefined });
    Object.defineProperty(video, 'getVideoPlaybackQuality', { value: undefined });

    const media = mediaOf(video);

    expect(media.requestVideoFrameCallback).toBeUndefined();
    expect(media.cancelVideoFrameCallback).toBeUndefined();
    expect(media.getVideoPlaybackQuality).toBeUndefined();
  });

  test('<video> が持てば、<video> に渡す', async () => {
    const { mediaOf } = await import('./playerMedia');
    const video = document.createElement('video');
    const request = vi.fn(() => 7);
    const cancel = vi.fn();
    const quality = { totalVideoFrames: 30, droppedVideoFrames: 2 };
    Object.defineProperty(video, 'requestVideoFrameCallback', { value: request });
    Object.defineProperty(video, 'cancelVideoFrameCallback', { value: cancel });
    Object.defineProperty(video, 'getVideoPlaybackQuality', { value: () => quality });
    const callback = () => {};

    const media = mediaOf(video);

    expect(media.requestVideoFrameCallback?.(callback)).toBe(7);
    expect(request).toHaveBeenCalledWith(callback);
    media.cancelVideoFrameCallback?.(7);
    expect(cancel).toHaveBeenCalledWith(7);
    expect(media.getVideoPlaybackQuality?.()).toBe(quality);
  });
});

// FakeAudioContext は、jsdom にない AudioContext の代わり。createMediaElementSource を呼んだ回数を数える。
class FakeAudioContext {
  static created = 0;
  sources = 0;
  readonly destination = { kind: 'destination' };
  constructor() {
    FakeAudioContext.created += 1;
  }
  createMediaElementSource() {
    // 本物は、1 つの <video> に 2 度目を呼ぶと例外を投げる。
    this.sources += 1;

    return { kind: 'source' };
  }
}

describe('audioPort', () => {
  test('音の口は 1 度だけ作り、音量を上げる段と同じ AudioContext を使う', async () => {
    FakeAudioContext.created = 0;
    vi.stubGlobal('AudioContext', FakeAudioContext);
    const { mediaOf } = await import('./playerMedia');
    const { sharedAudioContext } = await import('./sharedAudioContext');
    const video = document.createElement('video');

    const first = mediaOf(video).audioPort();
    const second = mediaOf(video).audioPort();

    expect(second).toBe(first);
    expect(first.context).toBe(sharedAudioContext());
    expect((first.context as unknown as FakeAudioContext).sources).toBe(1);
    expect(first.sink).toBe(first.context.destination);
    expect(FakeAudioContext.created).toBe(1);
  });
});
