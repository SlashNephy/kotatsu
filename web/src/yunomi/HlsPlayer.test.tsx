import { act, cleanup, render } from '@testing-library/react';
import { createRef } from 'react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { HlsPlayer } from './HlsPlayer';

// FakeHls は、hls.js の代わり。作られたものを instances に残し、読み込みを止めたか、読み直したかを数える。
const fake = vi.hoisted(() => {
  class FakeHls {
    static Events = { FRAG_PARSING_METADATA: 'meta', MANIFEST_PARSED: 'parsed', ERROR: 'error' };
    static instances: FakeHls[] = [];
    stopLoad = vi.fn();
    startLoad = vi.fn();
    destroy = vi.fn();
    on = vi.fn();
    loadSource = vi.fn();
    attachMedia = vi.fn();
    constructor() {
      FakeHls.instances.push(this);
    }
  }

  return { FakeHls };
});

vi.mock('hls.js', () => ({ default: fake.FakeHls }));
vi.mock('./captions', () => ({
  createCaptions: () => ({
    setVisible: () => {},
    destroy: () => {},
    reset: () => {},
    feedID3: () => {},
  }),
}));

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  fake.FakeHls.instances = [];
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

async function renderLive() {
  const videoRef = createRef<HTMLVideoElement | null>() as { current: HTMLVideoElement | null };
  render(<HlsPlayer url="/live/playlist.m3u8" videoRef={videoRef} live captions={false} />);
  await vi.waitFor(() => expect(fake.FakeHls.instances).toHaveLength(1));
  const video = videoRef.current;
  if (video === null) {
    throw new Error('no video');
  }

  return video;
}

describe('HlsPlayer', () => {
  test('ライブを一時停止したら、読み込みを止め、すぐに再開したら続きから読む', async () => {
    // 読み続けると、kyusu はセグメントを要求されないのでセッションを止め、プレイリストの要求でまた起動する (#534)。
    const video = await renderLive();
    const hls = fake.FakeHls.instances[0];

    act(() => {
      video.dispatchEvent(new Event('pause'));
    });
    expect(hls?.stopLoad).toHaveBeenCalledTimes(1);

    vi.setSystemTime(Date.now() + 3_000);
    act(() => {
      video.dispatchEvent(new Event('play'));
    });
    expect(hls?.startLoad).toHaveBeenCalledTimes(1);
    expect(fake.FakeHls.instances).toHaveLength(1);
  });

  test('ライブを長く一時停止してから再開したら、最新の位置から読み直す', async () => {
    // kyusu はセッションを止めているので、続きのセグメントはもうない。
    const video = await renderLive();
    const first = fake.FakeHls.instances[0];

    act(() => {
      video.dispatchEvent(new Event('pause'));
    });
    vi.setSystemTime(Date.now() + 60_000);
    act(() => {
      video.dispatchEvent(new Event('play'));
    });

    await vi.waitFor(() => expect(fake.FakeHls.instances).toHaveLength(2));
    expect(first?.destroy).toHaveBeenCalled();
    expect(first?.startLoad).not.toHaveBeenCalled();
  });
});
