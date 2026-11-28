import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import type { PlayerMedia } from './playerMedia';
import { useVodTimeline, type VodTimelineOptions } from './useVodTimeline';

// FakeMedia は、読み込んだ範囲と、復号のエラーを持たせられる video の代わり。
class FakeMedia extends EventTarget {
  currentTime = 0;
  readyState = HTMLMediaElement.HAVE_ENOUGH_DATA;
  seeking = false;
  error: { code: number } | null = null;
  loadedTo = 0;

  get buffered(): TimeRanges {
    const end = this.loadedTo;

    return { length: end > 0 ? 1 : 0, start: () => 0, end: () => end } as TimeRanges;
  }

  seek(t: number) {
    this.currentTime = t;
  }
}

// renderTimeline は、600 秒の再生元を、100 秒から変換の経路で読む。
function renderTimeline(options: Partial<VodTimelineOptions> = {}) {
  const media = new FakeMedia();
  const mediaRef = { current: media as unknown as PlayerMedia };
  const hook = renderHook(
    (props: Partial<VodTimelineOptions>) =>
      useVodTimeline({
        streamUrl: '/stream',
        media: media as unknown as PlayerMedia,
        mediaRef,
        convert: true,
        hls: false,
        initial: 100,
        duration: 600,
        ...props,
      }),
    { initialProps: options },
  );

  return { media, ...hook };
}

describe('useVodTimeline', () => {
  beforeEach(() => {
    vi.useFakeTimers({ now: Date.parse('2026-11-28T00:00:00Z') });
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise<Response>(() => {})),
    );
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  test('切れたら、通知を出さずに今の位置から読み直し、すぐに続けて切れたらエラーにする', () => {
    const { result } = renderTimeline();
    const first = result.current.stream.url;

    act(() => result.current.onError());
    expect(result.current.error).toBeNull();
    expect(result.current.stream.url).not.toBe(first);

    act(() => result.current.onError());
    expect(result.current.error).not.toBeNull();

    // やり直したら、エラーを消して、同じ位置から読み直す。
    act(() => result.current.retry());
    expect(result.current.error).toBeNull();
    expect(result.current.position).toBe(100);
  });

  test.each([
    {
      name: '長さが決まっていれば、終わりに届く前に終わった配信を読み直す',
      endKnown: true,
      restarted: true,
    },
    {
      name: 'インデックスを作っている間は、終わりを見分けずに読み直さない',
      endKnown: false,
      restarted: false,
    },
  ])('$name', ({ endKnown, restarted }) => {
    const { result, media } = renderTimeline({ endKnown });
    media.loadedTo = 50;
    const first = result.current.stream.url;

    act(() => result.current.onConvertState('completed'));

    expect(result.current.stream.url !== first).toBe(restarted);
  });

  test('読み直すときは、変換と kyusu の両方の経路を、今の位置から読み始めさせる', () => {
    const { result } = renderTimeline();

    act(() => result.current.restartAt(300));

    expect(result.current.stream.url).toContain('start=300.0');
    expect(result.current.hlsStream.start).toBe(300);
  });

  test.each([
    { name: '移れる範囲の外へは移らない', target: 900, clampSeek: undefined, want: 600 },
    {
      name: '再生元の決めた範囲に収める',
      target: 500,
      clampSeek: (s: number) => Math.min(s, 200),
      want: 200,
    },
  ])('$name', ({ target, clampSeek, want }) => {
    const { result } = renderTimeline(clampSeek === undefined ? {} : { clampSeek });

    let moved = 0;
    act(() => {
      moved = result.current.jumpTo(target);
    });

    expect(moved).toBe(want);
  });

  test('読み直す位置を、再生元の決めた位置に寄せる', () => {
    const { result } = renderTimeline({ restartFrom: (s) => Math.min(s, 598) });

    act(() => result.current.restartAt(600));

    expect(result.current.stream.url).toContain('start=598.0');
  });

  test('映像が自分でシークする経路では、配信を読み直さずに映像の時刻を移す', () => {
    const { result, media } = renderTimeline({ native: true });
    const url = result.current.stream.url;

    let moved = 0;
    act(() => {
      moved = result.current.jumpTo(250);
    });

    expect(moved).toBe(250);
    expect(media.currentTime).toBe(250);
    expect(result.current.stream.url).toBe(url);
    expect(result.current.hlsStream.loads).toBe(0);
  });

  test('映像が自分でシークする経路で読み直したら、読み込んだあとに、読み直した位置へ移る', () => {
    const { result, media } = renderTimeline({ native: true });

    act(() => result.current.restartAt(320));
    expect(result.current.hlsStream.loads).toBe(1);
    media.currentTime = 0;
    act(() => result.current.onConvertLoaded());

    expect(media.currentTime).toBe(320);
  });

  test('映像が自分でシークする経路では、ファイルを読み終えても、切れたとみなさない', () => {
    const { result, media } = renderTimeline({ native: true });
    media.loadedTo = 10;

    act(() => result.current.onConvertState('completed'));

    expect(result.current.hlsStream.loads).toBe(0);
    expect(result.current.error).toBeNull();
  });
});
