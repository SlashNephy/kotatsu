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

describe('フレームごとの呼び返しと、フレームの数', () => {
  // 部品は、メソッドの有無で requestAnimationFrame などに切り替える。対応していない <video> でも
  // メソッドがあると、切り替えずに呼んで、例外になる。
  test('<video> が持たなければ、持たない', async () => {
    const { mediaOf } = await import('./playerMedia');
    const video = document.createElement('video');
    Object.defineProperty(video, 'requestVideoFrameCallback', { value: undefined });
    Object.defineProperty(video, 'getVideoPlaybackQuality', { value: undefined });

    const media = mediaOf(video);

    // 値が undefined のプロパティもあってはならない。'requestVideoFrameCallback' in media で判定する部品があるため。
    expect('requestVideoFrameCallback' in media).toBe(false);
    expect('cancelVideoFrameCallback' in media).toBe(false);
    expect('getVideoPlaybackQuality' in media).toBe(false);
  });

  test('<video> が持てば、<video> に渡す', async () => {
    const { mediaOf } = await import('./playerMedia');
    const video = document.createElement('video');
    let pending: ((now: number, metadata: { mediaTime: number }) => void) | null = null;
    const request = vi.fn((next: typeof pending) => {
      pending = next;

      return 7;
    });
    const cancel = vi.fn();
    const quality = { totalVideoFrames: 30, droppedVideoFrames: 2 };
    Object.defineProperty(video, 'requestVideoFrameCallback', { value: request });
    Object.defineProperty(video, 'cancelVideoFrameCallback', { value: cancel });
    Object.defineProperty(video, 'getVideoPlaybackQuality', { value: () => quality });
    const callback = vi.fn();

    const media = mediaOf(video);

    const id = media.requestVideoFrameCallback?.(callback);
    expect(request).toHaveBeenCalledTimes(1);
    pending!(100, { mediaTime: 1.5 });
    expect(callback).toHaveBeenCalledWith(100, { mediaTime: 1.5 });
    const second = media.requestVideoFrameCallback?.(callback);
    expect(second).not.toBe(id);
    media.cancelVideoFrameCallback?.(second!);
    expect(cancel).toHaveBeenCalledWith(7);
    expect(media.getVideoPlaybackQuality?.()).toBe(quality);
  });

  // フォーク (#2287) でデコードしている間は、<video> がフレームを出さないので、<video> の呼び返しは来ない。
  test('canvas に描いている間は、画面の書き換えごとに、今の位置で呼び返し、frame は返さない', async () => {
    const { mediaOf, setCanvasPictures } = await import('./playerMedia');
    const video = document.createElement('video');
    const request = vi.fn(() => 7);
    Object.defineProperty(video, 'requestVideoFrameCallback', { value: request });
    Object.defineProperty(video, 'cancelVideoFrameCallback', { value: vi.fn() });
    Object.defineProperty(video, 'currentTime', { value: 12.5 });
    const frames: FrameRequestCallback[] = [];
    vi.stubGlobal('requestAnimationFrame', (next: FrameRequestCallback) => frames.push(next));
    const cancelAnimation = vi.fn();
    vi.stubGlobal('cancelAnimationFrame', cancelAnimation);
    const callback = vi.fn();
    const media = mediaOf(video);
    expect(media.frame).toBe(video);

    setCanvasPictures(video, { capture: () => Promise.resolve(video) });
    const id = media.requestVideoFrameCallback?.(callback);
    frames[0]!(200);

    expect(request).not.toHaveBeenCalled();
    expect(callback).toHaveBeenCalledWith(200, { mediaTime: 12.5 });
    expect(media.frame).toBeNull();
    media.requestVideoFrameCallback?.(callback);
    media.cancelVideoFrameCallback?.(id! + 1);
    expect(cancelAnimation).toHaveBeenCalledWith(2);
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
