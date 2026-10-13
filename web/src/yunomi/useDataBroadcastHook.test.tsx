import { act, cleanup, render } from '@testing-library/react';
import { useRef } from 'react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import classes from './TsPlayer.module.css';
import { RemoteKey, useDataBroadcast } from './useDataBroadcast';

// FakeBrowser は、BMLBrowser の代わり。イベントを外から送り、映像の位置の有無を決められる。
const fake = vi.hoisted(() => {
  class FakeBrowser extends EventTarget {
    static instances: FakeBrowser[] = [];
    readonly mediaElement: HTMLElement;
    // videoPlane は、いまのページの映像の位置。null なら、映像の位置がないページ。
    videoPlane: HTMLElement | null = document.createElement('object');
    // pressed は、押したリモコンのボタン (keydown) の順。
    readonly pressed: number[] = [];
    readonly content = {
      processKeyDown: (key: number) => {
        this.pressed.push(key);
      },
      processKeyUp: () => {},
    };

    constructor(options: { mediaElement: HTMLElement }) {
      super();
      this.mediaElement = options.mediaElement;
      FakeBrowser.instances.push(this);
    }

    getVideoElement() {
      return this.videoPlane;
    }

    emitMessage() {}

    destroy() {}
  }

  return { FakeBrowser };
});

vi.mock('web-bml', () => ({ BMLBrowser: fake.FakeBrowser }));

// FakeWorker は、Web Worker の代わり。読み終えたことを、外から知らせられる。
class FakeWorker extends EventTarget {
  static last: FakeWorker | null = null;

  constructor() {
    super();
    FakeWorker.last = this;
  }

  postMessage() {}

  terminate() {}

  send(data: unknown) {
    this.dispatchEvent(new MessageEvent('message', { data }));
  }
}

beforeEach(() => {
  fake.FakeBrowser.instances = [];
  players.length = 0;
  vi.stubGlobal('Worker', FakeWorker);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// players は、Harness が作った video。データ放送の中へ移ると、文書から探せなくなるので、ここで持つ。
const players: HTMLVideoElement[] = [];

// Harness は、TsPlayer と同じく、映像の要素 (.media) の中に video を置き、データ放送の層を重ねる。
function Harness({ shown = true }: { shown?: boolean }) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const { layer, state } = useDataBroadcast(
    { url: '/streams/services/3273601024/data-broadcast', serviceId: 1024 },
    videoRef,
    shown,
  );

  return (
    <div>
      <div
        className={classes.media}
        data-testid="media"
        ref={(element) => {
          if (element !== null && videoRef.current === null) {
            const video = document.createElement('video');
            element.append(video);
            videoRef.current = video;
            players.push(video);
          }
        }}
      />
      {layer}
      <span data-testid="state">{state}</span>
    </div>
  );
}

const load = new CustomEvent('load', { detail: { resolution: { width: 960, height: 540 } } });

describe('useDataBroadcast', () => {
  test('出していない間も裏で受け取り、出したら d ボタンで入る', () => {
    vi.useFakeTimers();
    try {
      const { rerender } = render(<Harness shown={false} />);
      const browser = fake.FakeBrowser.instances[0];
      expect(browser).toBeDefined();
      act(() => {
        browser?.dispatchEvent(load);
        browser?.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        vi.advanceTimersByTime(5000);
      });
      // 出していない間は、押さず、映像もプレイヤーに残す。
      expect(browser?.pressed).toEqual([]);
      expect(players[0]?.isConnected && browser?.mediaElement.contains(players[0])).toBe(false);

      rerender(<Harness shown />);
      act(() => {
        vi.advanceTimersByTime(0);
      });
      expect(browser?.pressed).toEqual([RemoteKey.d]);
    } finally {
      vi.useRealTimers();
    }
  });

  test('出すのをやめても受け取りは続け、見えている文書なら d ボタンで最初の文書へ戻す', () => {
    vi.useFakeTimers();
    try {
      const { rerender } = render(<Harness shown />);
      const browser = fake.FakeBrowser.instances[0];
      act(() => {
        browser?.dispatchEvent(load);
        browser?.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        vi.advanceTimersByTime(0);
        browser?.dispatchEvent(load);
        browser?.dispatchEvent(new CustomEvent('invisible', { detail: false }));
      });
      expect(browser?.pressed).toEqual([RemoteKey.d]);

      rerender(<Harness shown={false} />);
      expect(fake.FakeBrowser.instances).toHaveLength(1);
      expect(browser?.pressed).toEqual([RemoteKey.d, RemoteKey.d]);

      // 戻った見えない文書から、次に出すときも d ボタンで入る。
      act(() => {
        browser?.dispatchEvent(load);
        browser?.dispatchEvent(new CustomEvent('invisible', { detail: true }));
      });
      rerender(<Harness shown />);
      act(() => {
        vi.advanceTimersByTime(0);
      });
      expect(browser?.pressed).toEqual([RemoteKey.d, RemoteKey.d, RemoteKey.d]);
    } finally {
      vi.useRealTimers();
    }
  });

  test.each([
    {
      name: '見えない文書を読んだら、文書が見えるようになるまで d ボタンを押し直す',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(load);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        vi.advanceTimersByTime(600);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: false }));
        vi.advanceTimersByTime(5000);
      },
      wantPresses: 4,
    },
    {
      name: '同じ見えない文書を読み直しても、待たずに押し直しを続ける',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(load);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        vi.advanceTimersByTime(400);
        browser.dispatchEvent(load);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        vi.advanceTimersByTime(200);
      },
      wantPresses: 4,
    },
    {
      name: '最初から見える文書なら、押さない',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(load);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: false }));
        vi.advanceTimersByTime(5000);
      },
      wantPresses: 0,
    },
    {
      name: '見えるようにならなくても、押し直すのは決まった回数まで',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(load);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        vi.advanceTimersByTime(60_000);
      },
      wantPresses: 100,
    },
  ])('$name', ({ steps, wantPresses }) => {
    vi.useFakeTimers();
    try {
      render(<Harness />);
      const browser = fake.FakeBrowser.instances[0];
      expect(browser).toBeDefined();
      act(() => {
        if (browser !== undefined) {
          steps(browser);
        }
      });

      expect(browser?.pressed).toEqual(Array.from({ length: wantPresses }, () => RemoteKey.d));
    } finally {
      vi.useRealTimers();
    }
  });

  test.each([
    {
      name: '文書を表示したら、映像をデータ放送の中へ移す',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(load);
      },
      wantInBroadcast: true,
      wantState: 'shown',
    },
    {
      name: '文書が隠れている間に読み込んでも、映像はプレイヤーに残す',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        browser.dispatchEvent(load);
      },
      wantInBroadcast: false,
      wantState: 'shown',
    },
    {
      name: '映像の位置がないページを表示し直しても、映像はプレイヤーに残す',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.videoPlane = null;
        browser.dispatchEvent(load);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        browser.dispatchEvent(new CustomEvent('invisible', { detail: false }));
      },
      wantInBroadcast: false,
      wantState: 'shown',
    },
    {
      name: '文書がまた表示されたら、映像をデータ放送の中へ戻す',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(load);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        browser.dispatchEvent(new CustomEvent('invisible', { detail: false }));
      },
      wantInBroadcast: true,
      wantState: 'shown',
    },
    {
      name: 'TS を読めなくなったら、映像をプレイヤーへ戻す',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(load);
        FakeWorker.last?.send({ type: 'kotatsu:end', error: true });
      },
      wantInBroadcast: false,
      wantState: 'ended',
    },
  ])('$name', ({ steps, wantInBroadcast, wantState }) => {
    const { getByTestId } = render(<Harness />);
    const [browser] = fake.FakeBrowser.instances;
    if (browser === undefined) {
      throw new Error('BMLBrowser is not created');
    }

    act(() => steps(browser));

    const video = players.at(-1) ?? null;
    expect(browser.mediaElement.contains(video)).toBe(wantInBroadcast);
    expect(getByTestId('media').contains(video)).toBe(!wantInBroadcast);
    expect(getByTestId('state').textContent).toBe(wantState);
  });
});
