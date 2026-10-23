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
    handlers = new Map<string, () => void>();
    on = vi.fn((event: string, handler: () => void) => {
      this.handlers.set(event, handler);
    });
    // emit は、hls.js がイベントを出したことにする。
    emit(event: string) {
      this.handlers.get(event)?.();
    }
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
    setSuperimposeVisible: () => {},
    setStyle: () => {},
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

async function renderPlayer(live: boolean) {
  const videoRef = createRef<HTMLVideoElement | null>() as { current: HTMLVideoElement | null };
  render(<HlsPlayer url="/playlist.m3u8" videoRef={videoRef} live={live} captions={false} />);
  await vi.waitFor(() => expect(fake.FakeHls.instances).toHaveLength(1));
  const video = videoRef.current;
  if (video === null) {
    throw new Error('no video');
  }
  // jsdom は再生しないので、play を何もしないものにする。
  video.play = () => Promise.resolve();
  act(() => fake.FakeHls.instances[0]?.emit('parsed'));

  return video;
}

const renderLive = () => renderPlayer(true);

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

    // 作り直すと、前の hls.js が video から外れるので、ブラウザは pause を出す。これで新しい hls.js の読み込みを止めない。
    const second = fake.FakeHls.instances[1];
    act(() => {
      video.dispatchEvent(new Event('pause'));
    });
    act(() => second?.emit('parsed'));
    expect(second?.stopLoad).not.toHaveBeenCalled();
  });

  test('録画は、一時停止しても読み込みを止めない', async () => {
    // 録画の仮想 HLS は、kyusu がプレイリストを要求されても起動しないので、読み続けてよい。
    const video = await renderPlayer(false);
    const hls = fake.FakeHls.instances[0];

    act(() => {
      video.dispatchEvent(new Event('pause'));
    });
    vi.setSystemTime(Date.now() + 60_000);
    act(() => {
      video.dispatchEvent(new Event('play'));
    });

    expect(hls?.stopLoad).not.toHaveBeenCalled();
    expect(hls?.startLoad).not.toHaveBeenCalled();
    expect(fake.FakeHls.instances).toHaveLength(1);
  });
});
