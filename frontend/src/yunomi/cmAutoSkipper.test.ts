import { describe, expect, test } from 'vitest';

import {
  CmAutoSkipper,
  coverTimeoutMs,
  uncoverFrames,
  type SkipperFrame,
  type SkipperHost,
} from './cmAutoSkipper';
import { cmLookahead } from './cmSkip';

// fakeHost は、CmAutoSkipper が動かした先を、起きた順に記録する。
function fakeHost(overrides: Partial<SkipperHost> = {}) {
  const events: string[] = [];
  const timers: { callback: () => void; ms: number; cancelled: boolean }[] = [];
  const state = { enabled: true, scrubbing: false, paused: false, lastSeek: null as number | null };
  const host: SkipperHost = {
    enabled: () => state.enabled,
    scrubbing: () => state.scrubbing,
    paused: () => state.paused,
    lastSeek: () => state.lastSeek,
    sections: () => [{ from: 600, to: 645 }],
    cover: (kind) => events.push(`cover:${kind}`),
    uncover: () => events.push('uncover'),
    jump: (target) => {
      events.push(`jump:${target}`);

      return target;
    },
    notify: () => events.push('notify'),
    setTimer: (callback, ms) => {
      const timer = { callback, ms, cancelled: false };
      timers.push(timer);

      return () => {
        timer.cancelled = true;
      };
    },
    ...overrides,
  };

  return { host, events, timers, state };
}

const playing = (position: number | null, extra: Partial<SkipperFrame> = {}): SkipperFrame => ({
  position,
  seeking: false,
  paused: false,
  presented: true,
  ...extra,
});

describe('CmAutoSkipper', () => {
  test('先読みの分だけ手前で、映っている本編のコマを写した蓋を出してから、続いた CM の終わりへ飛ぶ', () => {
    const { host, events } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(599.5));
    expect(events).toEqual([]);
    skipper.frame(playing(600 - cmLookahead / 2));
    expect(events).toEqual(['cover:still', 'jump:645', 'notify']);
  });

  test('CM の中のコマで飛ぶときは、写さずに黒で覆う', () => {
    const { host, events } = fakeHost();
    new CmAutoSkipper(host).frame(playing(610));
    expect(events).toEqual(['cover:black', 'jump:645', 'notify']);
  });

  test('飛び先以上のコマが続けて映るまで、蓋を外さない', () => {
    const { host, events } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(610));
    events.length = 0;
    // 読み直しの途中のコマ、seek の途中のコマ、I フレームから飛び先の手前までのコマでは数えない。
    skipper.frame(playing(null));
    skipper.frame(playing(645, { seeking: true }));
    skipper.frame(playing(644.9));
    for (let i = 0; i < uncoverFrames - 1; i++) {
      skipper.frame(playing(645 + i / 30));
    }
    expect(events).toEqual([]);
    skipper.frame(playing(645 + uncoverFrames / 30));
    expect(events).toEqual(['uncover']);
    expect(skipper.waiting).toBe(false);
  });

  test('飛び先以上のコマの途中で手前のコマが来たら、数え直す', () => {
    const { host, events } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(610));
    events.length = 0;
    for (let i = 0; i < uncoverFrames - 1; i++) {
      skipper.frame(playing(645));
    }
    skipper.frame(playing(644));
    for (let i = 0; i < uncoverFrames - 1; i++) {
      skipper.frame(playing(645));
    }
    expect(events).toEqual([]);
  });

  test('timeupdate からの知らせは、currentTime が先へ進んでいても、蓋を外す数に入れない', () => {
    const { host, events } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(610));
    events.length = 0;
    for (let i = 0; i < uncoverFrames + 2; i++) {
      skipper.frame(playing(645, { presented: false }));
    }
    expect(events).toEqual([]);
  });

  test('止めている間に届いたコマは、飛び先以上でも、蓋を外す数に入れない', () => {
    // requestVideoFrameCallback のないブラウザは、requestAnimationFrame で代えるので、止めている間もコマの知らせが届く。
    const { host, events } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(610));
    events.length = 0;
    for (let i = 0; i < uncoverFrames + 2; i++) {
      skipper.frame(playing(645, { paused: true }));
    }
    expect(events).toEqual([]);
    for (let i = 0; i < uncoverFrames; i++) {
      skipper.frame(playing(645 + i / 30));
    }
    expect(events).toEqual(['uncover']);
  });

  test('飛び先が映るのを待っている間は、もう一度は飛ばない', () => {
    const { host, events } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(610));
    skipper.frame(playing(620));
    expect(events.filter((event) => event.startsWith('jump'))).toEqual(['jump:645']);
  });

  test('外す条件を満たさないまま上限の時間がたったら、蓋を外す', () => {
    const { host, events, timers } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(610));
    expect(timers.map((timer) => timer.ms)).toEqual([coverTimeoutMs]);
    timers[0]!.callback();
    expect(events.at(-1)).toBe('uncover');
    expect(skipper.waiting).toBe(false);
  });

  test('止めている間は、上限の時間がたっても外さず、再生してから上限がたったら外す', () => {
    const { host, events, timers, state } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.holdUntil(645);
    state.paused = true;
    timers[0]!.callback();
    expect(events).toEqual(['cover:black']);
    expect(skipper.waiting).toBe(true);
    state.paused = false;
    timers[1]!.callback();
    expect(events).toEqual(['cover:black', 'uncover']);
  });

  test('外したら、外し忘れのタイマーを止める', () => {
    const { host, timers } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(610));
    for (let i = 0; i < uncoverFrames; i++) {
      skipper.frame(playing(645));
    }
    expect(timers[0]!.cancelled).toBe(true);
  });

  test('実際に移った先 (録画の終わりで止めた位置) を、飛び先として待つ', () => {
    const { host, events } = fakeHost({ jump: () => 640 });
    const skipper = new CmAutoSkipper(host);
    skipper.frame(playing(610));
    for (let i = 0; i < uncoverFrames; i++) {
      skipper.frame(playing(640));
    }
    expect(events.at(-1)).toBe('uncover');
  });

  test.each([
    { name: '一時停止の間', frame: playing(610, { paused: true }), state: {} },
    {
      name: '位置がまだ信じられない間 (作り直しや読み直しの途中)',
      frame: playing(null),
      state: {},
    },
    { name: 'シークバーをつまんでいる間', frame: playing(610), state: { scrubbing: true } },
    { name: '設定を切っている間', frame: playing(610), state: { enabled: false } },
    { name: '自分で区間の中へシークしたとき', frame: playing(610), state: { lastSeek: 605 } },
  ])('$name は飛ばない', ({ frame, state: overrides }) => {
    const { host, events, state } = fakeHost();
    Object.assign(state, overrides);
    new CmAutoSkipper(host).frame(frame);
    expect(events).toEqual([]);
  });

  test('相対の移動の行き先が CM の中なら、行き先へは移らず、黒い蓋を出して CM の終わりへ飛ぶ', () => {
    const { host, events } = fakeHost();
    expect(new CmAutoSkipper(host).skipFrom(610)).toBe(true);
    expect(events).toEqual(['cover:black', 'jump:645', 'notify']);
  });

  test.each([
    { name: '行き先が本編', position: 650, state: {} },
    { name: '設定を切っている', position: 610, state: { enabled: false } },
  ])('相対の移動で、$name ときは何もしない', ({ position, state: overrides }) => {
    const { host, events, state } = fakeHost();
    Object.assign(state, overrides);
    expect(new CmAutoSkipper(host).skipFrom(position)).toBe(false);
    expect(events).toEqual([]);
  });

  test('開き始めは、飛ばずに黒い蓋で覆い、求めた位置以上のコマが映るまで待つ', () => {
    const { host, events } = fakeHost();
    const skipper = new CmAutoSkipper(host);
    skipper.holdUntil(645);
    expect(events).toEqual(['cover:black']);
    // 配信は手前の I フレームから流れるので、CM の末尾のコマが来ても、飛ばずに隠したまま待つ。
    skipper.frame(playing(644.6));
    for (let i = 0; i < uncoverFrames; i++) {
      skipper.frame(playing(645 + i / 30));
    }
    expect(events).toEqual(['cover:black', 'uncover']);
  });
});
