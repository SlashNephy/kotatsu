import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { useSeekableStream } from './useSeekableStream';

// FakeVideo は、timeupdate を送り、時刻を書き換えられる video の代わり。
class FakeVideo extends EventTarget {
  currentTime = 0;
  readyState = HTMLMediaElement.HAVE_ENOUGH_DATA;

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
        useSeekableStream('/stream', element as unknown as HTMLVideoElement, 100),
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
    // 映っている映像は 133 秒まで進んだ。新しい読み込みは 129.5 秒から流れ、最初のコマの時刻は 0.4 秒。
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

  test('裏で読み込んでいる間に、もう一度知らせが届いても、映っている映像の起点で数え続ける', async () => {
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
