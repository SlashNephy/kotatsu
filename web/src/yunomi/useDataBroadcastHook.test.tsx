import { act, cleanup, render } from '@testing-library/react';
import { useRef } from 'react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import classes from './TsPlayer.module.css';
import { useDataBroadcast } from './useDataBroadcast';

// FakeBrowser は、BMLBrowser の代わり。イベントを外から送り、映像の位置の有無を決められる。
const fake = vi.hoisted(() => {
  class FakeBrowser extends EventTarget {
    static instances: FakeBrowser[] = [];
    readonly mediaElement: HTMLElement;
    // videoPlane は、いまのページの映像の位置。null なら、映像の位置がないページ。
    videoPlane: HTMLElement | null = document.createElement('object');
    readonly content = { processKeyDown: () => {}, processKeyUp: () => {} };

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
  vi.stubGlobal('Worker', FakeWorker);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// players は、Harness が作った video。データ放送の中へ移ると、文書から探せなくなるので、ここで持つ。
const players: HTMLVideoElement[] = [];

// Harness は、TsPlayer と同じく、映像の要素 (.media) の中に video を置き、データ放送の層を重ねる。
function Harness() {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const { layer, state } = useDataBroadcast(
    { url: '/streams/services/3273601024/data-broadcast', serviceId: 1024 },
    videoRef,
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
