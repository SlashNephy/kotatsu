import { act, cleanup, render } from '@testing-library/react';
import { createRef, type RefObject } from 'react';
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

function player(url: string | null, videoRef: RefObject<HTMLVideoElement | null>, live: boolean) {
  return <HlsPlayer url={url} videoRef={videoRef} live={live} captions={false} />;
}

async function renderPlayer(live: boolean) {
  const videoRef = createRef<HTMLVideoElement | null>() as { current: HTMLVideoElement | null };
  const { rerender } = render(player('/playlist.m3u8', videoRef, live));
  await vi.waitFor(() => expect(fake.FakeHls.instances).toHaveLength(1));
  const video = videoRef.current;
  if (video === null) {
    throw new Error('no video');
  }
  // jsdom は再生しないので、play は呼ばれた回数だけを数える。
  const play = vi.fn(() => Promise.resolve());
  video.play = play;
  act(() => fake.FakeHls.instances[0]?.emit('parsed'));
  // reload は、URL を変えて読み直させる。parse が false なら、プレイリストを読み終えないままにする。
  // next で、読み直すときにライブかどうかを変えられる。
  const reload = async (url: string, parse = true, next = live) => {
    const count = fake.FakeHls.instances.length;
    rerender(player(url, videoRef, next));
    await vi.waitFor(() => expect(fake.FakeHls.instances).toHaveLength(count + 1));
    if (parse) {
      act(() => fake.FakeHls.instances.at(-1)?.emit('parsed'));
    }
  };

  // unload は、URL を null にして、読むのをやめさせる。
  const unload = () => rerender(player(null, videoRef, live));

  return { video, play, reload, unload };
}

// setPaused は、video が止まっているか、終わりまで再生したかを決める。jsdom の video は、いつも止まっている。
function setPaused(video: HTMLVideoElement, paused: boolean, ended = false) {
  Object.defineProperty(video, 'paused', { configurable: true, get: () => paused });
  Object.defineProperty(video, 'ended', { configurable: true, get: () => ended });
}

const renderLive = () => renderPlayer(true);

describe('HlsPlayer', () => {
  test('ライブを一時停止したら、読み込みを止め、すぐに再開したら続きから読む', async () => {
    // 読み続けると、kyusu はセグメントを要求されないのでセッションを止め、プレイリストの要求でまた起動する (#534)。
    const { video } = await renderLive();
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
    const { video } = await renderLive();
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
    const { video } = await renderPlayer(false);
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

  test.each([
    {
      name: '一時停止していたら、一時停止のまま待つ',
      paused: true,
      ended: false,
      playing: 0,
      autoplay: false,
    },
    {
      name: '再生していたら、再生を続ける',
      paused: false,
      ended: false,
      playing: 1,
      autoplay: true,
    },
    {
      name: '終わりまで再生して止まっていたら、再生する',
      paused: true,
      ended: true,
      playing: 1,
      autoplay: true,
    },
  ])('録画を読み直したとき、$name', async ({ paused, ended, playing, autoplay }) => {
    // 一時停止している間に配信が切れて今の位置から読み直しても、勝手に再生を始めない (#1969)。
    const { video, play, reload } = await renderPlayer(false);
    play.mockClear();
    setPaused(video, paused, ended);

    await reload('/playlist.m3u8#1');

    expect(play).toHaveBeenCalledTimes(playing);
    expect(video.autoplay).toBe(autoplay);
  });

  test('録画を一時停止したまま、読み終える前にもう一度読み直しても、一時停止のまま待つ', async () => {
    const { video, play, reload } = await renderPlayer(false);
    play.mockClear();
    setPaused(video, true);

    await reload('/playlist.m3u8#1', false);
    await reload('/playlist.m3u8#2');

    expect(play).not.toHaveBeenCalled();
    expect(video.autoplay).toBe(false);
  });

  test('録画を一時停止したまま読むのをやめたら、次に読むときは再生する', async () => {
    // 読まない間に、別の録画やライブへ移ることがある。前の一時停止を引き継がない。
    const { video, play, reload, unload } = await renderPlayer(false);
    play.mockClear();
    setPaused(video, true);

    unload();
    await reload('/other.m3u8');

    expect(play).toHaveBeenCalledTimes(1);
    expect(video.autoplay).toBe(true);
  });

  test('録画を一時停止したままライブに変えたら、再生する', async () => {
    // ライブ視聴で、バッファの再生からライブの端に戻ると、live が false から true に変わる。
    const { video, play, reload } = await renderPlayer(false);
    play.mockClear();
    setPaused(video, true);

    await reload('/live.m3u8', true, true);

    expect(play).toHaveBeenCalledTimes(1);
    expect(video.autoplay).toBe(true);
  });

  test('ライブは、止めたまま読み直したら再生する', async () => {
    // ライブで止めたまま読み直すのは、チャンネルか画質を変えたときだけ。
    const { video, play, reload } = await renderLive();
    play.mockClear();
    setPaused(video, true);

    await reload('/live.m3u8');

    expect(play).toHaveBeenCalledTimes(1);
    expect(video.autoplay).toBe(true);
  });
});
