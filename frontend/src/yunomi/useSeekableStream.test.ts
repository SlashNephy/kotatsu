import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useSeekableStream } from './useSeekableStream';
import type { PlayerMedia } from './playerMedia';

// FakeVideo は、timeupdate を送り、時刻を書き換えられる video の代わり。
class FakeVideo extends EventTarget {
  currentTime = 0;
  readyState = HTMLMediaElement.HAVE_ENOUGH_DATA;
  seeking = false;
  // loadedTo は、読み込んである範囲の終わり (video の時刻)。0 なら、何も読み込んでいない。
  loadedTo = 0;

  get buffered(): TimeRanges {
    const end = this.loadedTo;

    return {
      length: end > 0 ? 1 : 0,
      start: () => 0,
      end: () => end,
    } as TimeRanges;
  }

  seek(t: number) {
    this.currentTime = t;
  }

  at(t: number) {
    this.currentTime = t;
    this.dispatchEvent(new Event('timeupdate'));
  }

  // playTo は、1 秒ずつ t まで流す。一度に 5 秒より進めると、途切れた区間を越えたとみなされる。
  playTo(t: number) {
    while (this.currentTime + 1 < t) {
      this.at(this.currentTime + 1);
    }
    this.at(t);
  }
}

// streamStarts は、HEAD の start= ごとに返す、流し始めた位置。
function mockHead(streamStarts: Record<string, number>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const start = new URL(url, 'http://localhost').searchParams.get('start') ?? '';
      const at = streamStarts[start];

      return Promise.resolve(
        new Response(null, {
          status: 200,
          headers: at === undefined ? {} : { 'X-Kotatsu-Start': String(at) },
        }),
      );
    }),
  );
}

describe('useSeekableStream', () => {
  beforeEach(() => {
    // 100 秒から読んで 99 秒から流れた。100 秒から読み直すと、99.5 秒から流れる。
    mockHead({ '100.0': 99, '130.0': 129.5 });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // playFrom は、100 秒から読み、video の時刻 2 秒から 30 秒流した (位置は 99 + 30 = 129 秒) 状態にする。
  async function playFrom(video: FakeVideo) {
    const hook = renderHook(
      ({ element }: { element: FakeVideo }) =>
        useSeekableStream('/stream', element as unknown as PlayerMedia, 100),
      { initialProps: { element: video } },
    );
    await act(async () => {});
    act(() => hook.result.current.onLoaded());
    act(() => video.at(2));
    act(() => video.playTo(32));

    return hook;
  }

  test('裏で読み込んでいる間も、映っている映像で位置を進める', async () => {
    const video = new FakeVideo();
    const { result } = await playFrom(video);
    expect(result.current.position).toBe(129);

    act(() => result.current.restartAt(130, true));
    await act(async () => {});
    act(() => video.playTo(35));
    expect(result.current.position).toBe(132);
  });

  test('映ったフレームの位置は、位置の基準を決めてから返し、読み直す間は返さない', async () => {
    const video = new FakeVideo();
    const { result } = await playFrom(video);
    // 99 秒から流れ、video の時刻 2 秒を基準にしたので、31.5 秒のフレームは 128.5 秒。
    expect(result.current.framePosition(31.5)).toBe(128.5);

    act(() => result.current.restartAt(130));
    expect(result.current.framePosition(32)).toBeNull();
  });

  test('読み直す (シークする) 間は、位置を止める', async () => {
    const video = new FakeVideo();
    const { result } = await playFrom(video);

    act(() => result.current.restartAt(130));
    await act(async () => {});
    act(() => video.playTo(35));
    expect(result.current.position).toBe(130);
  });

  test('切り替える前に、映っている映像が進んだ分だけ進める先を返し、切り替えた後も続きから数える', async () => {
    const video = new FakeVideo();
    const { result, rerender } = await playFrom(video);
    act(() => result.current.restartAt(130, true));
    await act(async () => {});
    // 映っている映像は 133 秒まで進んだ。新しい読み込みは 129.5 秒から流れ、最初のフレームの時刻は 0.4 秒。
    act(() => video.playTo(36));
    expect(result.current.handoverTarget(0.4)).toBeCloseTo(0.4 + (133 - 129.5));

    // 3.9 秒へ進めた映像に切り替え、2 秒流した。
    const next = new FakeVideo();
    next.currentTime = 3.9;
    act(() => result.current.onLoaded({ base: 0.4, current: 3.9 }));
    rerender({ element: next });
    act(() => next.playTo(5.9));
    expect(result.current.position).toBeCloseTo(135);
  });

  test('裏で読み込んでいる間に、もう一度通知が届いても、映っている映像の起点で数え続ける', async () => {
    mockHead({ '100.0': 99, '130.0': 129.5, '133.0': 132.5 });
    const video = new FakeVideo();
    const { result } = await playFrom(video);
    act(() => result.current.restartAt(130, true));
    await act(async () => {});
    act(() => video.playTo(35));
    act(() => result.current.restartAt(result.current.positionRef.current, true));
    await act(async () => {});
    act(() => video.playTo(37));
    expect(result.current.position).toBe(134);
  });

  test('求めた位置より手前のフレームが映っている間は、位置を動かさない', async () => {
    const video = new FakeVideo();
    const { result } = renderHook(() =>
      useSeekableStream('/stream', video as unknown as PlayerMedia, 100),
    );
    await act(async () => {});
    act(() => result.current.onLoaded());
    // 99 秒から流れ、video の時刻 2 秒が最初のフレーム。100 秒に当たるのは、video の時刻 3 秒。
    act(() => video.at(2));
    act(() => video.at(2.5));
    expect(result.current.position).toBe(100);
    expect(result.current.framePosition(2.5)).toBeNull();

    act(() => video.at(3));
    expect(result.current.position).toBe(100);
    act(() => video.at(4));
    expect(result.current.position).toBe(101);
  });

  test('求めた位置まで読み込んであれば、手前のフレームを飛ばしてその位置へ進める', async () => {
    const video = new FakeVideo();
    video.loadedTo = 10;
    const { result } = renderHook(() =>
      useSeekableStream('/stream', video as unknown as PlayerMedia, 100),
    );
    await act(async () => {});
    act(() => result.current.onLoaded());
    act(() => video.at(2));
    act(() => video.at(2.2));

    expect(video.currentTime).toBe(3);
  });

  test('流し始めた位置を引けなくても、求めた位置から数える', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('offline'))),
    );
    const video = new FakeVideo();
    const { result } = renderHook(() =>
      useSeekableStream('/stream', video as unknown as PlayerMedia, 100),
    );
    await act(async () => {});
    expect(result.current.settled).toBe(true);
    act(() => result.current.onLoaded());
    act(() => video.at(2));
    act(() => video.at(5));
    expect(result.current.position).toBe(103);
  });

  test('新しい読み込みが流し始めた位置が分かるまでは、進める先を返さない', async () => {
    const video = new FakeVideo();
    const { result } = await playFrom(video);
    // HEAD がまだ答えていない。
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise<Response>(() => {})),
    );
    act(() => result.current.restartAt(130, true));
    expect(result.current.handoverTarget(0.4)).toBeNull();
  });
});
