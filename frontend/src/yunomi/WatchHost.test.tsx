import { act, cleanup, render, screen } from '@testing-library/react';
import { type ReactNode, useState } from 'react';
import { MantineProvider } from '@mantine/core';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { saveMiniPlayer } from './miniPlayer';
import { mediaOf } from './playerMedia';

// 小さなプレイヤーを押したときの移り先を覚える。ルーターは立てない。
const navigated: unknown[] = [];
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => (options: unknown) => {
    navigated.push(options);
  },
}));
import { browserStorage } from './volume';
import {
  useHostedWatch,
  useWatchMedia,
  useWatchMini,
  WatchHost,
  WatchHostProvider,
} from './WatchHost';

afterEach(() => {
  cleanup();
  saveMiniPlayer(browserStorage(), true);
  replaceVideo = () => {};
});

// Counter は、押した回数を覚える。置き場で描き直しても状態が残るかを確かめる。
function Counter({ label }: { label: string }) {
  const [count, setCount] = useState(0);

  return (
    <button type="button" onClick={() => setCount(count + 1)}>
      {label}:{count}
    </button>
  );
}

// Route は、ルートの代わり。useHostedWatch に渡した要素を、ルートの位置に返す。
function Route({ label }: { label: string }) {
  return <div data-testid="route">{useHostedWatch(<Counter label={label} />)}</div>;
}

// App は、アプリの根元の代わり。ルートと置き場を並べる。
function App({ route }: { route: ReactNode }) {
  return (
    <MantineProvider env="test">
      <WatchHostProvider>
        {route}
        <div data-testid="host">
          <WatchHost />
        </div>
      </WatchHostProvider>
    </MantineProvider>
  );
}

// replaceVideo は、描いている Player の映像を、プレイヤーがつなぎ直したときのように替える。
let replaceVideo: (video: HTMLVideoElement) => void = () => {};

// Player は、視聴の画面の代わり。再生中かを playing で決める映像と、小さく描いているかを出す。
// 視聴の画面と同じく、映している映像の経路を置き場に覚えさせる。
function Player({ playing }: { playing: boolean }) {
  const mini = useWatchMini();
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  replaceVideo = setVideo;
  useWatchMedia(video === null ? null : mediaOf(video));

  return (
    <div>
      <video
        ref={(element) => {
          if (element !== null) {
            // jsdom の映像は再生できないので、再生中かだけを決める。
            Object.defineProperty(element, 'paused', { configurable: true, value: !playing });
            setVideo((current) => current ?? element);
          }
        }}
      />
      <span data-testid="mode">{mini ? 'mini' : 'page'}</span>
    </div>
  );
}

function PlayerRoute({ playing }: { playing: boolean }) {
  return <>{useHostedWatch(<Player playing={playing} />)}</>;
}

describe('小さなプレイヤー', () => {
  test('再生中にルートを離れたら、閉じずに小さく描き、閉じる操作で閉じる', () => {
    const { rerender } = render(<App route={<PlayerRoute playing />} />);
    expect(screen.getByTestId('mode').textContent).toBe('page');

    rerender(<App route={null} />);
    expect(screen.getByTestId('mode').textContent).toBe('mini');

    // 押すと、視聴の画面へ戻る。
    act(() => {
      screen.getAllByRole('button', { name: 'player.mini.open' })[0]?.click();
    });
    expect(navigated).toEqual([{ href: window.location.pathname }]);

    act(() => {
      screen.getByRole('button', { name: 'player.mini.close' }).click();
    });
    expect(screen.getByTestId('host').textContent).toBe('');
  });

  test('小さなプレイヤーの一時停止のボタンで、映像を止める', () => {
    const { rerender } = render(<App route={<PlayerRoute playing />} />);
    const video = screen.getByTestId('host').querySelector('video');
    const pause = vi.fn();
    if (video !== null) {
      Object.defineProperty(video, 'pause', { configurable: true, value: pause });
    }
    rerender(<App route={null} />);

    act(() => {
      screen.getByRole('button', { name: 'player.mini.pause' }).click();
    });
    expect(pause).toHaveBeenCalledOnce();
  });

  test('小さく描いている間に映像が作り直されたら、新しい映像を止める (#1730)', () => {
    const { rerender } = render(<App route={<PlayerRoute playing />} />);
    rerender(<App route={null} />);
    const old = screen.getByTestId('host').querySelector('video');
    const oldPause = vi.fn();
    if (old !== null) {
      Object.defineProperty(old, 'pause', { configurable: true, value: oldPause });
    }
    act(() => {
      screen.getByRole('button', { name: 'player.mini.pause' }).click();
    });
    expect(oldPause).toHaveBeenCalledOnce();

    // プレイヤーがつなぎ直すと、裏で読み込んだ組の映像に替え、視聴の画面は新しい映像の経路を覚えさせ直す。
    // 裏で読み込んでいる間は、プレイヤーは古い映像のまま知らせる (TsPlayer の onVideo)。
    const next = document.createElement('video');
    Object.defineProperty(next, 'paused', { configurable: true, value: false });
    const pause = vi.fn();
    Object.defineProperty(next, 'pause', { configurable: true, value: pause });
    act(() => replaceVideo(next));

    act(() => {
      screen.getByRole('button', { name: 'player.mini.pause' }).click();
    });
    expect(pause).toHaveBeenCalledOnce();
    expect(oldPause).toHaveBeenCalledOnce();
  });

  test('止めていたら、ルートを離れたときに閉じる', () => {
    const { rerender } = render(<App route={<PlayerRoute playing={false} />} />);
    rerender(<App route={null} />);

    expect(screen.getByTestId('host').textContent).toBe('');
  });

  test('小さなプレイヤーを切っていたら、再生中でも閉じる', () => {
    saveMiniPlayer(browserStorage(), false);
    const { rerender } = render(<App route={<PlayerRoute playing />} />);
    rerender(<App route={null} />);

    expect(screen.getByTestId('host').textContent).toBe('');
  });

  test('視聴の画面に戻ったら、作り直さずに大きく描く', () => {
    const { rerender } = render(<App route={<PlayerRoute playing />} />);
    const video = screen.getByTestId('host').querySelector('video');
    rerender(<App route={null} />);
    rerender(<App route={<PlayerRoute playing />} />);

    expect(screen.getByTestId('mode').textContent).toBe('page');
    expect(screen.getByTestId('host').querySelector('video')).toBe(video);
  });
});

describe('useHostedWatch', () => {
  test('ルートの位置ではなく、置き場で描く', () => {
    render(<App route={<Route label="live" />} />);

    expect(screen.getByTestId('host').textContent).toBe('live:0');
    expect(screen.getByTestId('route').textContent).toBe('');
  });

  test('ルートが描き直しても、置き場の要素は作り直さず、状態を保つ', () => {
    const { rerender } = render(<App route={<Route label="live" />} />);
    act(() => {
      screen.getByRole('button').click();
    });
    expect(screen.getByRole('button').textContent).toBe('live:1');

    rerender(<App route={<Route label="other" />} />);
    expect(screen.getByRole('button').textContent).toBe('other:1');
  });

  test('ルートを離れたら、置き場から下ろす', () => {
    const { rerender } = render(<App route={<Route label="live" />} />);
    rerender(<App route={null} />);

    expect(screen.getByTestId('host').textContent).toBe('');
  });

  test('置き場がなければ、その場で描く', () => {
    render(<Route label="live" />);

    expect(screen.getByTestId('route').textContent).toBe('live:0');
  });
});
