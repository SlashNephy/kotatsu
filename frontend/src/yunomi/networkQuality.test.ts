import { describe, expect, test } from 'vitest';

import {
  audioStep,
  chooseDown,
  createNetworkController,
  defaultNetworkParams,
  probeBytes,
  type NetworkController,
  type NetworkDecision,
  type NetworkInput,
  type NetworkMode,
  type Step,
} from './networkQuality';

const params = defaultNetworkParams;

// ladder は、HD の番組の段階のテーブル。オリジナルのビットレートは分からない。kyusu の段階は、映像のビットレートに音声の 128 kbps を足したもの。
const ladder: Step[] = [
  { name: 'original', bitrate: null },
  { name: '1080p', bitrate: 6_128_000 },
  { name: '720p', bitrate: 3_628_000 },
  { name: '540p', bitrate: 2_128_000 },
  { name: '360p', bitrate: 1_128_000 },
  { name: audioStep, bitrate: 128_000 },
];

// healthy は、無事なときの計測した値。
const healthy: NetworkInput = {
  active: true,
  started: true,
  waiting: false,
  lowReadyState: false,
  frameAgeMs: 33,
  ahead: 5,
  broadcastLag: 10,
  receiveRate: null,
  server: null,
};

type Logged = { at: number; decision: NetworkDecision };

// Sim は、判定のたびに計測した値を渡し、決めたとおりに段階を動かす。時刻は 0 から始める。
class Sim {
  now = 0;
  current: string;
  automatic = true;
  log: Logged[] = [];
  readonly controller: NetworkController;

  constructor(
    current: string,
    public mode: NetworkMode = 'probe',
    readonly steps: Step[] = ladder,
  ) {
    this.current = current;
    this.controller = createNetworkController({ now: 0 });
  }

  // run は、ms のあいだ判定を繰り返す。input は、判定の時刻を受けて、無事なときとの違いを返す。
  run(ms: number, input: Partial<NetworkInput> | ((now: number) => Partial<NetworkInput>) = {}) {
    const end = this.now + ms;
    while (this.now + params.tickMs <= end) {
      this.now += params.tickMs;
      const diff = typeof input === 'function' ? input(this.now) : input;
      const decision = this.controller.tick(
        this.now,
        { ...healthy, ...diff },
        {
          ladder: this.steps,
          current: this.current,
          automatic: this.automatic,
          mode: this.mode,
        },
      );
      if (decision.kind !== 'none') {
        this.log.push({ at: this.now, decision });
      }
      if (decision.kind === 'step') {
        this.current = decision.to;
      }
    }

    return this;
  }

  // steps は、段階を動かした記録。
  moves() {
    return this.log.flatMap(({ at, decision }) =>
      decision.kind === 'step'
        ? [{ at, to: decision.to, direction: decision.direction, reason: decision.reason }]
        : [],
    );
  }

  probes() {
    return this.log.flatMap(({ at, decision }) =>
      decision.kind === 'probe' ? [{ at, bytes: decision.bytes }] : [],
    );
  }
}

// need は、段階のテーブルのビットレートどおりに、段階を流すのに要る速さを返す。オリジナルは既定値。
const need = (step: Step) => step.bitrate ?? params.defaultTopBitrate;

describe('chooseDown', () => {
  test.each([
    {
      name: '受け取りの速さ × 余白の係数に収まる最も高い段',
      current: 'original',
      rate: 3_000_000,
      want: '540p',
    },
    {
      name: '今の段に収まっても、今より下の段にする',
      current: '720p',
      rate: 10_000_000,
      want: '540p',
    },
    {
      name: '収まる段が無ければ、映像のある最も低い段',
      current: '1080p',
      rate: 500_000,
      want: '360p',
    },
    { name: '速さが分からなければ 1 段下げる', current: '720p', rate: null, want: '540p' },
    { name: 'オリジナルからも 1 段', current: 'original', rate: null, want: '1080p' },
    { name: '映像のある最も低い段からは下げない', current: '360p', rate: 100_000, want: null },
    { name: '音声だけからは下げない', current: audioStep, rate: null, want: null },
  ])('$name', ({ current, rate, want }) => {
    expect(chooseDown(ladder, current, rate, need, params)).toBe(want);
  });
});

describe('probeBytes', () => {
  test.each([
    {
      name: '(次の段 × 余裕の倍率 − 今の受け取り) で試しの量の時間かかる量',
      need: 2_128_000,
      rate: 1_128_000,
      // (2,128,000 × 1.3 − 1,128,000) × 2 ÷ 8 = 409,600
      want: 409_600,
    },
    { name: '下限に丸める', need: 1_128_000, rate: 1_000_000, want: params.probeMinBytes },
    { name: '上限に丸める', need: 20_000_000, rate: 0, want: params.probeMaxBytes },
    {
      name: '受け取りの速さが分からなければ 0 とみなす',
      need: 2_000_000,
      rate: null,
      want: 650_000,
    },
  ])('$name', ({ need: needed, rate, want }) => {
    expect(probeBytes(needed, rate, params)).toBe(want);
  });
});

describe('足りない兆し', () => {
  // 切り替え直後の無視時間がたってから兆候を出し、下げた時刻を確認する。
  const start = params.ignoreAfterSwitchMs;
  test.each([
    {
      name: '詰まり (waiting) はすぐに数える',
      mode: 'probe' as const,
      input: { waiting: true },
      within: 500,
      want: true,
    },
    {
      name: 'readyState の低さは、詰まりとみなす時間に満たなければ数えない',
      mode: 'probe' as const,
      input: { lowReadyState: true },
      within: params.stallMs - params.tickMs,
      want: false,
    },
    {
      name: 'readyState の低さが、詰まりとみなす時間続いたら数える',
      mode: 'probe' as const,
      input: { lowReadyState: true },
      within: params.stallMs + params.tickMs,
      want: true,
    },
    {
      name: 'コマの途絶とみなす時間に満たなければ数えない',
      mode: 'probe' as const,
      input: { frameAgeMs: params.frameGapMs - 1, ahead: params.frameGapAheadBelow - 0.1 },
      within: 5_000,
      want: false,
    },
    {
      name: 'コマの途絶とみなす時間たっても次のコマが来なければ数える',
      mode: 'probe' as const,
      input: { frameAgeMs: params.frameGapMs, ahead: params.frameGapAheadBelow - 0.1 },
      within: 500,
      want: true,
    },
    {
      name: '先読みが十分にあれば、コマの途絶を数えない (変換か描画の側の途切れ、#2033)',
      mode: 'rate' as const,
      input: { frameAgeMs: params.frameGapMs * 2, ahead: params.frameGapAheadBelow },
      within: 5_000,
      want: false,
    },
    {
      name: '測らないとき (音声だけなど) のコマの途絶は数えない',
      mode: 'probe' as const,
      input: { frameAgeMs: null, ahead: params.frameGapAheadBelow - 0.1 },
      within: 5_000,
      want: false,
    },
    {
      name: 'サーバが塊を捨てた',
      mode: 'probe' as const,
      input: { server: { backlogBytes: null, dropped: true, encodeSpeed: null } },
      within: 500,
      want: true,
    },
    {
      name: 'エンコードが遅れている間の詰まりは、回線のせいにしない',
      mode: 'probe' as const,
      input: { waiting: true, server: { backlogBytes: null, dropped: false, encodeSpeed: 0.5 } },
      within: 3_000,
      want: false,
    },
    {
      name: 'エンコードの速さが 0 なら測れていないとみなし、詰まりを数える',
      mode: 'probe' as const,
      input: { waiting: true, server: { backlogBytes: null, dropped: false, encodeSpeed: 0 } },
      within: 500,
      want: true,
    },
    {
      name: '受け取りの速さの不足は、ライブ (試しの経路) では数えない',
      mode: 'probe' as const,
      input: { receiveRate: 1_000_000 },
      within: 20_000,
      want: false,
    },
  ])('$name', ({ mode, input, within, want }) => {
    const sim = new Sim('720p', mode).run(start).run(within, input);
    expect(sim.moves().length > 0).toBe(want);
  });

  test('切り替え直後の無視時間の間は数えない', () => {
    const sim = new Sim('720p').run(params.ignoreAfterSwitchMs - params.tickMs, { waiting: true });
    expect(sim.moves()).toEqual([]);
  });

  test('止めている間は数えない', () => {
    const sim = new Sim('720p').run(start).run(10_000, { active: false, waiting: true });
    expect(sim.moves()).toEqual([]);
  });

  test('サーバのエンコードが遅れ続けたら、段を下げず、遅れている旨を出す', () => {
    const slow = { backlogBytes: null, dropped: false, encodeSpeed: 0.5 };
    const sim = new Sim('720p')
      .run(start)
      .run(params.encodeSlowDurationMs, { server: slow })
      .run(5_000, { server: slow, frameAgeMs: 5_000 });
    expect(sim.moves()).toEqual([]);
    expect(sim.controller.status(sim.now).encoderLagging).toBe(true);
  });

  test.each([
    {
      name: '減少の速さの閾値以上で、減少の継続時間減り続けたら数える',
      from: 3.9,
      perSecond: 0.25,
      want: true,
    },
    {
      // 低遅延モードで 1.1 倍に速めているときの減り方。
      name: '速めているだけの減り方 (0.1 秒/秒) は数えない',
      from: 3.9,
      perSecond: 0.1,
      want: false,
    },
    { name: '見るバッファの量より多ければ数えない', from: 9, perSecond: 0.25, want: false },
  ])('バッファの減少: $name', ({ from, perSecond, want }) => {
    const sim = new Sim('720p').run(start, { ahead: from });
    const begin = sim.now;
    sim.run(params.drainDurationMs + 1_000, (now) => ({
      ahead: Math.max(from - ((now - begin) / 1_000) * perSecond, 0.5),
    }));
    expect(sim.moves().length > 0).toBe(want);
  });

  // saw は、2 秒ごとにチャンクが届き、届くたびに 1 秒増え、次のチャンクまで 1 秒/秒で減るバッファ (kyusu のセグメント)。
  // perSecond は、底の下がり方。
  test.each([
    { name: '塊が届くたびの上下 (のこぎりの歯) だけなら数えない', perSecond: 0, want: false },
    {
      name: 'のこぎりの歯でも、底が閾値以上の速さで下がり続けたら数える',
      perSecond: 0.3,
      want: true,
    },
  ])('バッファの減少: $name', ({ perSecond, want }) => {
    const saw = (elapsed: number) =>
      3.5 - (elapsed / 1_000) * perSecond + (1 - (elapsed % 2_000) / 1_000);
    const sim = new Sim('720p').run(start, { ahead: 3.5 });
    const begin = sim.now;
    sim.run(params.drainDurationMs + 2_000, (now) => ({ ahead: saw(now - begin) }));
    expect(sim.moves().length > 0).toBe(want);
  });

  test.each([
    {
      name: '窓の間に伸びの閾値以上伸びたら数える',
      perSecond: 0.1,
      mode: 'probe' as const,
      want: true,
    },
    {
      name: '伸びの閾値に満たなければ数えない',
      perSecond: 0.05,
      mode: 'probe' as const,
      want: false,
    },
    {
      name: '録画の再生 (受け取りの速さの経路) では見ない',
      perSecond: 0.1,
      mode: 'rate' as const,
      want: false,
    },
  ])('放送に対する遅れの伸び: $name', ({ perSecond, mode, want }) => {
    const sim = new Sim('720p', mode).run(start);
    const begin = sim.now;
    sim.run(params.lagWindowMs + 2_000, (now) => ({
      // キーフレームのチャンクで揺れても、区間の最小値で比較する。
      broadcastLag:
        10 + ((now - begin) / 1_000) * perSecond + ((now / params.tickMs) % 4 === 0 ? 0.8 : 0),
    }));
    expect(sim.moves().length > 0).toBe(want);
  });

  test('放送に対する遅れの伸び: 1 回の刻みで端が戻ったとみなす伸び以上伸びたら、測り直す', () => {
    const sim = new Sim('720p').run(start);
    const begin = sim.now;
    sim.run(params.lagWindowMs + 5_000, (now) => ({
      broadcastLag: now - begin >= 10_000 ? 10 + params.lagResetGrowth : 10,
    }));
    expect(sim.moves()).toEqual([]);
  });

  test.each([
    {
      name: 'バックログが継続時間のあいだ伸び続けたら数える',
      step: 1_000,
      ms: params.backlogDurationMs + 500,
      want: true,
    },
    { name: '伸びが止まったら数え直す', step: 0, ms: 10_000, want: false },
  ])('$name', ({ step, ms, want }) => {
    const sim = new Sim('720p').run(start);
    const begin = sim.now;
    sim.run(ms, (now) => ({
      server: {
        backlogBytes: 1_000 + ((now - begin) / params.tickMs) * step,
        dropped: false,
        encodeSpeed: null,
      },
    }));
    expect(sim.moves().length > 0).toBe(want);
  });

  test.each([
    {
      name: '受け取りの速さの不足が、速さ不足の継続時間続いたら数える',
      ms: params.rateWindowMs * params.rateShortWindows + params.tickMs,
      want: true,
    },
    {
      name: '速さ不足の継続時間に満たなければ数えない',
      ms: params.rateWindowMs * params.rateShortWindows - params.tickMs,
      want: false,
    },
  ])('録画の再生: $name', ({ ms, want }) => {
    // 720p (3.628 Mbps) に、4 Mbps × 0.8 = 3.2 Mbps では足りない。
    const sim = new Sim('720p', 'rate').run(start).run(ms, { receiveRate: 4_000_000 });
    expect(sim.moves().length > 0).toBe(want);
  });

  test('手で選んだ画質では段を動かさず、足りないことだけを返す', () => {
    const sim = new Sim('720p');
    sim.automatic = false;
    sim.run(start).run(1_000, { waiting: true });
    expect(sim.moves()).toEqual([]);
    expect(sim.controller.short()).toBe(true);
  });
});

describe('下げ方', () => {
  const start = params.ignoreAfterSwitchMs;

  test('受け取りの速さに収まる段へ下げる', () => {
    const sim = new Sim('original').run(start).run(500, { waiting: true, receiveRate: 3_000_000 });
    expect(sim.moves()).toEqual([
      { at: start + 500, to: '540p', direction: 'down', reason: 'bandwidth' },
    ]);
  });

  test('切り替え後の据え置きの間は、続けて下げない', () => {
    const sim = new Sim('720p')
      .run(start)
      .run(params.holdAfterSwitchMs, { waiting: true })
      .run(500, { waiting: true });
    expect(sim.moves().map((move) => [move.at, move.to])).toEqual([
      [start + 500, '540p'],
      [start + 500 + params.holdAfterSwitchMs, '360p'],
    ]);
  });

  test('枠不足のあとの据え置きの間は、下げ直さない', () => {
    const sim = new Sim('720p').run(start);
    sim.controller.noSlot(sim.now);
    sim.run(params.holdAfterNoSlotMs - 1_000, { waiting: true });
    expect(sim.moves()).toEqual([]);
    sim.run(1_000, { waiting: true });
    expect(sim.moves().map((move) => move.to)).toEqual(['540p']);
  });

  // 360p に移ってから、止まる (足りない) と少し動く (足りる) を同じ長さで繰り返す。
  const alternate = (period: number) => (now: number) => ({
    waiting: Math.floor(now / period) % 2 === 0,
  });
  test.each([
    {
      name: '止まる・少し動くの繰り返しでも、窓の割合で音声だけにする',
      steps: ladder,
      input: alternate(2_000),
      want: [audioStep],
    },
    {
      name: '足りない時間が音声だけにする割合に満たなければ、音声だけにしない',
      steps: ladder,
      input: (now: number) => ({ waiting: now % 6_000 < 2_000 }),
      want: [],
    },
    {
      name: '音声だけの段が無ければ (追っかけ)、音声だけにしない',
      steps: ladder.filter((step) => step.name !== audioStep),
      input: alternate(2_000),
      want: [],
    },
  ])('$name', ({ steps, input, want }) => {
    const sim = new Sim('360p', 'probe', steps).run(start).run(params.audioWindowMs + 5_000, input);
    expect(sim.moves().map((move) => move.to)).toEqual(want);
  });

  test('最も低い段に移ってから、音声だけを考えるまでの時間がたつまでは、音声だけにしない', () => {
    const sim = new Sim('540p').run(start).run(500, { waiting: true });
    expect(sim.current).toBe('360p');
    sim.run(params.audioConsiderMs - 1_000, { waiting: true });
    expect(sim.current).toBe('360p');
  });
});

describe('試しのデータで上げる (ライブ)', () => {
  const start = params.ignoreAfterSwitchMs;

  test('足りない兆しが無い状態が試すまでの無事の時間続いたら試し、余裕があれば 1 段上げる', () => {
    const sim = new Sim('540p').run(start).run(params.probeAfterOkMs, { receiveRate: 2_128_000 });
    expect(sim.probes()).toEqual([
      { at: start + params.probeAfterOkMs, bytes: probeBytes(3_628_000, 2_128_000, params) },
    ]);
    expect(sim.controller.probing()).toBe(true);
    sim.controller.probed(sim.now, { kind: 'measured', bps: 3_000_000, alongsideBps: 2_128_000 });
    sim.run(500, { receiveRate: 2_128_000 });
    // プローブの 3 Mbps + プローブの間に映像が受け取った 2.128 Mbps ≥ 3.628 Mbps × 1.3
    expect(sim.moves()).toEqual([
      { at: sim.now, to: '720p', direction: 'up', reason: 'recovered' },
    ]);
  });

  // 回線が詰まっていると、プローブの間は映像が細る。プローブの前の受け取りの速さを足すと、同じ帯域を二重に数えて上げすぎる。
  test.each([
    {
      name: '試しの間に映像が受け取れなければ、試しの前の受け取りの速さを足さずに比べる',
      alongsideBps: 0,
      want: [],
    },
    { name: '試しの間に映像が受け取った速さは足す', alongsideBps: 1_000_000, want: ['720p'] },
  ])('$name', ({ alongsideBps, want }) => {
    const sim = new Sim('540p').run(start).run(params.probeAfterOkMs, { receiveRate: 3_100_000 });
    // 4 Mbps の回線: 試しが 4 Mbps を取った。3.628 Mbps × 1.3 = 4.72 Mbps
    sim.controller.probed(sim.now, { kind: 'measured', bps: 4_000_000, alongsideBps });
    sim.run(500, { receiveRate: 3_100_000 });
    expect(sim.moves().map((move) => move.to)).toEqual(want);
  });

  test('余裕が足りなければ上げず、待ちの倍率を掛けずにもう一度待つ', () => {
    const sim = new Sim('540p').run(start).run(params.probeAfterOkMs, { receiveRate: 2_128_000 });
    sim.controller.probed(sim.now, { kind: 'measured', bps: 1_000_000 });
    const probedAt = sim.now;
    sim.run(params.probeAfterOkMs + 500, { receiveRate: 2_128_000 });
    expect(sim.moves()).toEqual([]);
    expect(sim.probes().map((probe) => probe.at)).toEqual([
      probedAt,
      probedAt + 500 + params.probeAfterOkMs,
    ]);
  });

  test.each([
    {
      name: '5xx か打ち切りは、余裕が無かったとみなす',
      outcome: { kind: 'failed' as const },
      wantNextProbe: params.probeAfterOkMs + 500,
    },
    {
      name: '429 なら Retry-After の間は測らない',
      outcome: { kind: 'rateLimited' as const, retryAfterMs: 50_000 },
      wantNextProbe: 50_000,
    },
    {
      name: '429 に Retry-After が無ければ、試すまでの無事の時間だけ待つ',
      outcome: { kind: 'rateLimited' as const, retryAfterMs: null },
      wantNextProbe: params.probeAfterOkMs,
    },
  ])('$name', ({ outcome, wantNextProbe }) => {
    const sim = new Sim('540p').run(start).run(params.probeAfterOkMs);
    const probedAt = sim.now;
    sim.controller.probed(probedAt, outcome);
    sim.run(120_000);
    expect(sim.moves()).toEqual([]);
    expect(sim.probes()[1]?.at).toBe(probedAt + wantNextProbe);
  });

  test('測った値の有効期間を過ぎた値では上げず、測り直す', () => {
    const sim = new Sim('540p').run(start).run(params.probeAfterOkMs);
    sim.controller.probed(sim.now, { kind: 'measured', bps: 50_000_000 });
    // 有効期間のあいだ止めていて、判定しない。
    sim.run(params.probeValidityMs, { active: false });
    sim.run(500);
    expect(sim.moves()).toEqual([]);
  });

  test('音声だけからも、試しのデータで映像のある段へ戻す', () => {
    const sim = new Sim(audioStep)
      .run(start)
      .run(params.probeAfterOkMs, { frameAgeMs: null, receiveRate: 128_000 });
    sim.controller.probed(sim.now, { kind: 'measured', bps: 2_000_000 });
    sim.run(500, { frameAgeMs: null, receiveRate: 128_000 });
    expect(sim.moves().map((move) => move.to)).toEqual(['360p']);
  });

  // up は、540p から 720p へ上げ、上げた段階が映り出したところまで進めた Sim を返す。
  const up = () => {
    const sim = new Sim('540p').run(start).run(params.probeAfterOkMs);
    sim.controller.probed(sim.now, { kind: 'measured', bps: 10_000_000 });
    sim.run(500);
    expect(sim.current).toBe('720p');
    sim.run(2_000);
    sim.controller.shown(sim.now);

    return sim;
  };

  test('上げ直後の見張り時間のうちに足りなくなったら元の段へ戻し、次に試すまでの待ちに待ちの倍率を掛ける', () => {
    const sim = up();
    sim.run(params.holdAfterSwitchMs, { waiting: true });
    expect(sim.moves().at(-1)).toMatchObject({ to: '540p', direction: 'down', reason: 'upFailed' });
    const backAt = sim.moves().at(-1)!.at;
    sim.run(params.ignoreAfterSwitchMs + params.probeAfterOkMs * 2);
    expect(
      sim
        .probes()
        .filter((probe) => probe.at > backAt)
        .map((probe) => probe.at - backAt),
    ).toEqual([params.ignoreAfterSwitchMs + params.probeAfterOkMs * 2]);
  });

  test('上げた段で見張り時間のあいだ問題なく続けば、待ちの倍率を初期値に戻す', () => {
    const sim = up();
    sim.run(params.holdAfterSwitchMs, { waiting: true });
    // 戻した段階 (540p) から、2 倍の待ちで試し、また上げる。
    sim.run(params.ignoreAfterSwitchMs + params.probeAfterOkMs * 2 + 500);
    sim.controller.probed(sim.now, { kind: 'measured', bps: 10_000_000 });
    sim.run(500);
    expect(sim.current).toBe('720p');
    const upAt = sim.now;
    sim.controller.shown(upAt);
    sim.run(params.ignoreAfterSwitchMs + params.probeAfterOkMs);
    // 見張り時間 (30 秒) を過ぎたので、次の 1080p を試すまでの待ちは 1 倍に戻っている。
    expect(sim.probes().at(-1)!.at).toBe(upAt + params.ignoreAfterSwitchMs + params.probeAfterOkMs);
  });

  test('上げてから映るまでの上限がたっても映り出さなければ、元の段へ戻す', () => {
    const sim = new Sim('540p').run(start).run(params.probeAfterOkMs);
    sim.controller.probed(sim.now, { kind: 'measured', bps: 10_000_000 });
    sim.run(500);
    const upAt = sim.now;
    sim.run(params.upToDisplayMs);
    expect(sim.moves().at(-1)).toEqual({
      at: upAt + params.upToDisplayMs,
      to: '540p',
      direction: 'down',
      reason: 'upFailed',
    });
  });

  test('上げた段の再生が始まらないままでも、上げてから映るまでの上限がたてば元の段へ戻す', () => {
    const sim = new Sim('540p').run(start).run(params.probeAfterOkMs);
    sim.controller.probed(sim.now, { kind: 'measured', bps: 10_000_000 });
    sim.run(500);
    const upAt = sim.now;
    sim.run(params.upToDisplayMs, { started: false, waiting: true });
    expect(sim.moves().at(-1)).toEqual({
      at: upAt + params.upToDisplayMs,
      to: '540p',
      direction: 'down',
      reason: 'upFailed',
    });
  });

  test('再生が始まるまでの詰まりは数えない', () => {
    const sim = new Sim('720p').run(start + 20_000, {
      started: false,
      waiting: true,
      lowReadyState: true,
    });
    expect(sim.moves()).toEqual([]);
  });

  test('一時停止している間は、無事の時間に数えない', () => {
    const sim = new Sim('540p')
      .run(start)
      .run(params.probeAfterOkMs - 5_000)
      .run(10_000, { active: false })
      .run(params.probeAfterOkMs - 5_000);
    expect(sim.probes()).toEqual([]);
  });
});

describe('受け取りの速さで上げる (録画の再生)', () => {
  const start = params.ignoreAfterSwitchMs;

  test.each([
    {
      name: '次の段の上げる余裕の倍率以上が継続時間続けば 1 段上げる',
      rate: 5_500_000,
      ms: params.upDurationMs + 500,
      want: ['720p'],
    },
    {
      name: '継続時間に満たなければ上げない',
      rate: 5_500_000,
      ms: params.upDurationMs - 500,
      want: [],
    },
    { name: '倍率に満たなければ上げない', rate: 5_000_000, ms: 60_000, want: [] },
    { name: '速さが分からなければ上げない', rate: null, ms: 60_000, want: [] },
  ])('$name', ({ rate, ms, want }) => {
    // 720p は 3.628 Mbps。× 1.5 = 5.442 Mbps。
    const sim = new Sim('540p', 'rate').run(start).run(ms, { receiveRate: rate });
    expect(sim.moves().map((move) => move.to)).toEqual(want);
  });

  test('最高画質へは、その段が足りていた間に測った受け取りの速さと比べる', () => {
    const sim = new Sim('original', 'rate').run(start).run(5_000, { receiveRate: 12_000_000 });
    sim.run(params.rateWindowMs * params.rateShortWindows + 500, {
      receiveRate: 8_000_000,
      ahead: 3,
    });
    expect(sim.current).toBe('1080p');
    // 既定値 (20 Mbps × 1.5) より低くても、足りていた間の 12 Mbps × 1.5 = 18 Mbps を超えれば戻す。
    sim.run(start + params.holdAfterSwitchMs + params.upDurationMs, { receiveRate: 19_000_000 });
    expect(sim.moves().map((move) => move.to)).toEqual(['1080p', 'original']);
  });
});

test('経路の種類が変わったら (ライブと追っかけ)、溜めた兆しを捨ててしばらく数えない', () => {
  const controller = createNetworkController({ now: 0 });
  const context = { ladder, current: '720p', automatic: true };
  let now = params.ignoreAfterSwitchMs;
  controller.tick(now, healthy, { ...context, mode: 'probe' });
  now += params.tickMs;
  expect(controller.tick(now, { ...healthy, waiting: true }, { ...context, mode: 'rate' })).toEqual(
    {
      kind: 'none',
    },
  );
});

describe('下げた段が映り出さないとき', () => {
  const start = params.ignoreAfterSwitchMs;

  test('経路の種類が変わったら (ライブと追っかけ)、前の経路で下げた段の待ちを捨てる', () => {
    const sim = new Sim('720p').run(start).run(500, { waiting: true });
    sim.mode = 'rate';
    sim.run(params.downToDisplayMs + 500, { started: false });
    expect(sim.moves().map((move) => move.to)).toEqual(['540p']);
  });

  test.each([
    {
      name: '下げてから映るまでの上限がたっても映り出さなければ、もう一段下げる',
      from: '720p',
      input: { started: false },
      want: ['540p', '360p'],
    },
    {
      name: '受け取りの速さが分からなければ、最も低い段へ下げる',
      from: '1080p',
      input: { started: false },
      want: ['720p', '360p'],
    },
    {
      name: '受け取りの速さが分かれば、それに収まる段へ下げる',
      from: '1080p',
      input: { started: false, receiveRate: 3_000_000 },
      want: ['720p', '540p'],
    },
    {
      name: '上限の前に映り出せば、それ以上は下げない',
      from: '720p',
      input: (now: number) => ({ started: now >= start + 500 + params.downToDisplayMs - 1_000 }),
      want: ['540p'],
    },
    {
      name: '最も低い段で映り出さなければ、音声だけにする',
      from: '540p',
      input: { started: false },
      want: ['360p', audioStep],
    },
    {
      name: '一時停止している間は、下げない',
      from: '720p',
      input: { started: false, active: false },
      want: ['540p'],
    },
  ])('$name', ({ from, input, want }) => {
    // 詰まりで 1 段階下げた後、新しい段階が映り出さない。
    const sim = new Sim(from).run(start).run(500, { waiting: true });
    sim.run(params.downToDisplayMs + 500, input);
    expect(sim.moves().map((move) => move.to)).toEqual(want);
  });
});

describe('次に試すまでの時間', () => {
  const start = params.ignoreAfterSwitchMs;

  test.each([
    { name: '上に段があれば出す', current: '540p', automatic: true, want: true },
    { name: '最も高い段では出さない', current: 'original', automatic: true, want: false },
    { name: '手で選んだ画質では出さない', current: '540p', automatic: false, want: false },
  ])('$name', ({ current, automatic, want }) => {
    const sim = new Sim(current);
    sim.automatic = automatic;
    sim.run(start).run(5_000);
    expect(sim.controller.status(sim.now).nextProbeIn !== null).toBe(want);
  });

  test('試しのデータを取っている間は出さない', () => {
    const sim = new Sim('540p').run(params.ignoreAfterSwitchMs).run(params.probeAfterOkMs);
    expect(sim.controller.probing()).toBe(true);
    expect(sim.controller.status(sim.now).nextProbeIn).toBeNull();
  });
});

describe('周期的に途切れる回線 (帯域は足りている)', () => {
  const start = params.ignoreAfterSwitchMs;
  // 10 秒ごとに届き直し、その間は 1 秒に 0.35 秒ずつ減るバッファ。周期の終わりごとに、バッファの減少だけが立つ。
  const periodicDrain = (now: number) => ({ ahead: 3.5 - ((now % 10_000) / 1_000) * 0.35 });

  test.each([
    {
      name: 'バッファの減少だけなら、試すまでの無事を途切れさせず、試しのデータを取る',
      input: periodicDrain,
      want: 1,
    },
    {
      name: '詰まりは、試すまでの無事を途切れさせる',
      input: (now: number) => ({ waiting: now % 10_000 >= 9_000 }),
      want: 0,
    },
  ])('$name', ({ input, want }) => {
    const sim = new Sim('360p').run(start).run(params.probeAfterOkMs + 5_000, input);
    expect(
      sim.log.some(({ decision }) => decision.kind === 'step' && decision.direction === 'down'),
    ).toBe(false);
    expect(sim.probes()).toHaveLength(want);
  });
});
