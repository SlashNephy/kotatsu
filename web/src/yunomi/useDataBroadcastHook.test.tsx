import { act, cleanup, render } from '@testing-library/react';
import { useRef } from 'react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import classes from './TsPlayer.module.css';
import { settleMs } from './settleDelay';
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

    // options は、作るときに渡された設定。文書から呼ばれる API (選局など) を、テストから呼ぶのに使う。
    readonly options: {
      mediaElement: HTMLElement;
      epg?: { tune?: (networkId: number, transportStreamId: number, serviceId: number) => boolean };
      setMainAudioStreamCallback?: (componentId: number, channelId?: number) => boolean;
    };

    constructor(options: FakeBrowser['options']) {
      super();
      this.options = options;
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
function Harness({ shown = true, receive = true }: { shown?: boolean; receive?: boolean }) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const { layer, state, receiving } = useDataBroadcast(
    receive ? { url: '/streams/services/3273601024/data-broadcast', serviceId: 1024 } : null,
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
      <span data-testid="receiving">{String(receiving)}</span>
    </div>
  );
}

const load = new CustomEvent('load', { detail: { resolution: { width: 960, height: 540 } } });

describe('useDataBroadcast', () => {
  test('受け取るのをやめたあとは、出したり消したりしても、片付けた文書を操作しない', () => {
    vi.useFakeTimers();
    try {
      const { rerender } = render(<Harness shown={false} />);
      const browser = fake.FakeBrowser.instances[0];
      act(() => {
        browser?.dispatchEvent(load);
        browser?.dispatchEvent(new CustomEvent('invisible', { detail: false }));
      });
      // 視聴が切れたときなど、受け取るのをやめる。
      rerender(<Harness shown={false} receive={false} />);
      rerender(<Harness shown receive={false} />);
      rerender(<Harness shown={false} receive={false} />);
      act(() => {
        vi.advanceTimersByTime(5000);
      });

      expect(browser?.pressed).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
  });

  test('出してから、データ放送の画面を出せるまでを、受信中とする', () => {
    vi.useFakeTimers();
    try {
      const { getByTestId } = render(<Harness shown />);
      const browser = fake.FakeBrowser.instances[0];
      expect(getByTestId('receiving').textContent).toBe('true');
      act(() => {
        browser?.dispatchEvent(load);
        browser?.dispatchEvent(new CustomEvent('invisible', { detail: true }));
        vi.advanceTimersByTime(0);
      });
      expect(getByTestId('receiving').textContent).toBe('true');
      act(() => {
        browser?.dispatchEvent(new CustomEvent('invisible', { detail: false }));
      });
      // 受信中の知らせは、消すのを少し遅らせる (#445)。
      expect(getByTestId('receiving').textContent).toBe('true');
      act(() => {
        vi.advanceTimersByTime(settleMs);
      });
      expect(getByTestId('receiving').textContent).toBe('false');
    } finally {
      vi.useRealTimers();
    }
  });

  test('データ放送を閉じたら、受信中の知らせをすぐに消す', () => {
    const { getByTestId, rerender } = render(<Harness shown />);
    expect(getByTestId('receiving').textContent).toBe('true');

    rerender(<Harness shown={false} />);
    expect(getByTestId('receiving').textContent).toBe('false');
  });

  test('出していない間は、裏で受け取っていても受信中としない', () => {
    const { getByTestId } = render(<Harness shown={false} />);

    expect(getByTestId('receiving').textContent).toBe('false');
  });

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
      // web-bml は、読んだ文書が見えるかを load の直後に知らせる。隠れた文書なら、待たずに映像をプレイヤーへ戻す (#445)。
      name: '読んだ直後に隠れた文書と分かったら、待たずに映像をプレイヤーへ戻す',
      steps: (browser: InstanceType<typeof fake.FakeBrowser>) => {
        browser.dispatchEvent(load);
        browser.dispatchEvent(new CustomEvent('invisible', { detail: true }));
      },
      wantInBroadcast: false,
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
    vi.useFakeTimers();
    try {
      const { getByTestId } = render(<Harness />);
      const [browser] = fake.FakeBrowser.instances;
      if (browser === undefined) {
        throw new Error('BMLBrowser is not created');
      }

      // 見えるか隠れるかの変化は、落ち着いてから反映するので、その分を進める (#445)。
      act(() => {
        steps(browser);
        vi.advanceTimersByTime(settleMs);
      });

      const video = players.at(-1) ?? null;
      expect(browser.mediaElement.contains(video)).toBe(wantInBroadcast);
      expect(getByTestId('media').contains(video)).toBe(!wantInBroadcast);
      expect(getByTestId('state').textContent).toBe(wantState);
    } finally {
      vi.useRealTimers();
    }
  });

  describe('データ放送の接続だけが切れたら', () => {
    test.each([
      {
        name: '理由が分からなければ、5 秒後につなぎ直す',
        end: { error: true },
        after: 5000,
        browsers: 2,
      },
      { name: '5 秒たつまでは、つなぎ直さない', end: { error: true }, after: 4999, browsers: 1 },
      { name: '読み終えただけでも、つなぎ直す', end: { error: false }, after: 5000, browsers: 2 },
      { name: '5xx なら、つなぎ直す', end: { error: true, status: 503 }, after: 5000, browsers: 2 },
      {
        name: '4xx なら、つなぎ直さない',
        end: { error: true, status: 404 },
        after: 120_000,
        browsers: 1,
      },
    ])('$name', ({ end, after, browsers }) => {
      vi.useFakeTimers();
      try {
        render(<Harness shown />);
        act(() => {
          fake.FakeBrowser.instances[0]?.dispatchEvent(load);
          FakeWorker.last?.send({ type: 'kotatsu:end', ...end });
        });
        act(() => {
          vi.advanceTimersByTime(after);
        });

        expect(fake.FakeBrowser.instances).toHaveLength(browsers);
      } finally {
        vi.useRealTimers();
      }
    });

    test('つなぎ直せないうちは間隔を延ばし、文書を読めたら最初の間隔に戻す', () => {
      vi.useFakeTimers();
      try {
        render(<Harness shown />);
        const fail = () =>
          act(() => {
            FakeWorker.last?.send({ type: 'kotatsu:end', error: true });
          });
        const wait = (ms: number) =>
          act(() => {
            vi.advanceTimersByTime(ms);
          });

        fail();
        wait(5000);
        expect(fake.FakeBrowser.instances).toHaveLength(2);
        // 文書を読めないまま、また切れたら、10 秒待つ。
        fail();
        wait(9999);
        expect(fake.FakeBrowser.instances).toHaveLength(2);
        wait(1);
        expect(fake.FakeBrowser.instances).toHaveLength(3);
        // 文書を読めてから切れたら、また 5 秒で試す。
        act(() => {
          fake.FakeBrowser.instances[2]?.dispatchEvent(load);
        });
        fail();
        wait(5000);
        expect(fake.FakeBrowser.instances).toHaveLength(4);
      } finally {
        vi.useRealTimers();
      }
    });

    test('つなぎ直したら、出していれば、また d ボタンでデータ放送の画面に入る', () => {
      vi.useFakeTimers();
      try {
        render(<Harness shown />);
        act(() => {
          FakeWorker.last?.send({ type: 'kotatsu:end', error: true });
        });
        act(() => {
          vi.advanceTimersByTime(5000);
        });
        const browser = fake.FakeBrowser.instances[1];
        act(() => {
          browser?.dispatchEvent(load);
          vi.advanceTimersByTime(0);
        });

        expect(browser?.pressed).toContain(RemoteKey.d);
      } finally {
        vi.useRealTimers();
      }
    });

    test('出していなければ、つなぎ直しても d ボタンを押さない', () => {
      vi.useFakeTimers();
      try {
        render(<Harness shown={false} />);
        act(() => {
          FakeWorker.last?.send({ type: 'kotatsu:end', error: true });
        });
        act(() => {
          vi.advanceTimersByTime(5000);
        });
        const browser = fake.FakeBrowser.instances[1];
        act(() => {
          browser?.dispatchEvent(load);
          vi.advanceTimersByTime(1000);
        });

        expect(browser?.pressed).toEqual([]);
      } finally {
        vi.useRealTimers();
      }
    });

    test('切れたら、d ボタンを押し直すのをやめる', () => {
      vi.useFakeTimers();
      try {
        render(<Harness shown />);
        const browser = fake.FakeBrowser.instances[0];
        act(() => {
          browser?.dispatchEvent(load);
          vi.advanceTimersByTime(0);
        });
        act(() => {
          FakeWorker.last?.send({ type: 'kotatsu:end', error: true, status: 404 });
        });
        const pressed = browser?.pressed.length;
        act(() => {
          vi.advanceTimersByTime(3000);
        });

        expect(browser?.pressed).toHaveLength(pressed ?? 0);
      } finally {
        vi.useRealTimers();
      }
    });
  });
});

// TuneHarness は、データ放送の文書からの選局と音声の切り替えを受け取る。
function TuneHarness({
  onTune,
  onSelectAudio,
}: {
  onTune: (networkId: number, serviceId: number) => boolean;
  onSelectAudio: (componentTag: number, channelId?: number) => boolean;
}) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const { layer } = useDataBroadcast(
    { url: '/streams/services/3273601024/data-broadcast', serviceId: 1024 },
    videoRef,
    true,
    { onTune, onSelectAudio },
  );

  return (
    <div className={classes.media}>
      <video ref={videoRef} />
      {layer}
    </div>
  );
}

describe('データ放送の文書からの選局と音声の切り替え', () => {
  test('今の視聴の画面の処理に渡す。BMLBrowser を作り直さずに、新しい処理に切り替える', () => {
    const first = vi.fn(() => true);
    const second = vi.fn(() => false);
    const selectAudio = vi.fn(() => true);
    const { rerender } = render(<TuneHarness onTune={first} onSelectAudio={selectAudio} />);
    const [browser] = fake.FakeBrowser.instances;
    if (browser === undefined) {
      throw new Error('BMLBrowser is not created');
    }

    expect(browser.options.epg?.tune?.(32736, 32736, 1025)).toBe(true);
    expect(first).toHaveBeenCalledWith(32736, 1025);
    expect(browser.options.setMainAudioStreamCallback?.(0x11, 2)).toBe(true);
    expect(selectAudio).toHaveBeenCalledWith(0x11, 2);

    rerender(<TuneHarness onTune={second} onSelectAudio={selectAudio} />);
    expect(fake.FakeBrowser.instances).toHaveLength(1);
    expect(browser.options.epg?.tune?.(32736, 32736, 1024)).toBe(false);
    expect(second).toHaveBeenCalledWith(32736, 1024);
  });
});
