import { describe, expect, test, vi } from 'vitest';

import {
  createLiveBufferController,
  defaultLiveBufferParams as params,
  loadLiveBufferFloor,
  nextMode,
  prefersConservativeRate,
  saveLiveBufferFloor,
  type LiveBufferController,
  type RateMode,
} from './liveBuffer';

// arriveEvery は、from から to (ミリ秒) まで、interval ごとに塊が届いたことにする。届く直前のバッファは buffer 秒。
// 決め直しの刻みも、届くたびに進める。
function arriveEvery(
  controller: LiveBufferController,
  from: number,
  to: number,
  interval: number,
  buffer = 1,
): void {
  for (let at = from; at <= to; at += interval) {
    controller.arrive(at, buffer);
    controller.tick(at);
  }
}

// started は、選局から詰まりを数えない時間を過ぎ、0.5 秒おきに届いている制御を作る。時刻 10 秒まで進めてある。
function started(storedFloor: number | null = null, onFloorChange = vi.fn()) {
  const controller = createLiveBufferController({ now: 0, storedFloor, onFloorChange });
  arriveEvery(controller, 0, 10_000, 500);

  return { controller, onFloorChange };
}

describe('目標量の決め直し (止まったとき)', () => {
  test.each([
    {
      name: '空きで説明がつく詰まりでは、目標量を覆う量へ一度に上げ、下限は動かさない',
      // 2 秒の空きを覆うには、2 × 1.25 + 0.25 = 2.75 秒要る。
      gap: 2_000,
      want: { target: 2.75, floor: 1 },
    },
    {
      name: '説明がつかない詰まりでは、止まった高さの上に下限を置き、目標量を増分だけ足す',
      gap: 500,
      want: { target: 2, floor: 1.5 },
    },
    {
      name: '数える上限より長い空きは途絶えなので覆わず、説明がつかない詰まりとして扱う',
      gap: 4_000,
      want: { target: 2, floor: 1.5 },
    },
  ])('$name', ({ gap, want }) => {
    const { controller } = started();
    controller.arrive(10_000 + gap, 0);
    controller.arrive(10_000 + gap + 500, 0);

    expect(controller.stall(10_000 + gap + 600)).toBe(true);
    expect(controller.target).toBeCloseTo(want.target);
    expect(controller.floor).toBeCloseTo(want.floor);
  });

  // 0.5 秒おきに届き、届くたびに末尾が perArrival 秒伸びる。12 秒に止まる (空きは無い)。
  test.each([
    {
      name: '届いた映像の長さが経った時間に足りていれば、端末の詰まりとして下限を上げる',
      perArrival: 0.5,
      from: 0,
      want: { target: 2, floor: 1.5 },
    },
    {
      name: '受け取りの速さが放送のビットレートを下回っていれば、回線のせいとして下限は上げず、目標量だけ上げる',
      // 12 Mbps の回線で 16 Mbps の放送を受けたときの、0.75 倍の届き方。
      perArrival: 0.375,
      from: 0,
      want: { target: 2, floor: 1 },
    },
    {
      name: '比べる長さの半分より短い間しか測っていなければ、分からないとして下限を上げる',
      perArrival: 0.375,
      from: 12_000 - params.arrivalWindowMs / 2 + 500,
      want: { target: 2, floor: 1.5 },
    },
  ])('$name', ({ perArrival, from, want }) => {
    const controller = createLiveBufferController({ now: 0, storedFloor: null });
    for (let at = 0; at < 12_000; at += 500) {
      controller.arrive(at, 1, at >= from ? 100 + (at / 500) * perArrival : undefined);
      controller.tick(at);
    }

    expect(controller.stall(12_000)).toBe(true);
    expect(controller.target).toBeCloseTo(want.target);
    expect(controller.floor).toBeCloseTo(want.floor);
  });

  test('いま空いている最中の間隔は、数える上限までで覆う量にする', () => {
    const { controller } = started();
    // 最後に届いたのは 10 秒。20 秒まで届かないので、空きは上限の 3 秒で数える。
    expect(controller.stall(20_000)).toBe(true);
    expect(controller.target).toBeCloseTo(3 * 1.25 + 0.25);
    expect(controller.floor).toBe(1);
  });

  test('選局や飛んだ直後の詰まりは数えない', () => {
    const { controller } = started();
    controller.selfInflicted(10_000);

    expect(controller.stall(10_000 + params.selfInflictedGraceMs - 1)).toBe(false);
    expect(controller.target).toBe(1);
    expect(controller.stall(10_000 + params.selfInflictedGraceMs)).toBe(true);
    expect(controller.target).toBe(2);
  });

  test('目標量と下限は上限を超えない', () => {
    const { controller } = started();
    for (let i = 0; i < 10; i++) {
      controller.stall(10_000 + i * 10);
    }

    expect(controller.target).toBe(params.maxTarget);
    expect(controller.floor).toBe(params.maxTarget);
  });
});

describe('目標量の決め直し (止まっていないとき)', () => {
  // 時刻 t (ミリ秒) までの、0.5 秒おきに届いた後の目標量。10.1 秒に説明がつかない詰まりで、目標量 2、下限 1.5。
  test.each([
    { name: '縮め始めるまでは据え置く', at: 10_100 + params.shrinkAfterMs - 500, want: 2 },
    {
      name: '縮め始めたら、割合で縮め、下限より下にはしない',
      at: 10_100 + params.shrinkAfterMs + params.retargetIntervalMs * 3,
      // 2 → 1.8 → 1.62 → 1.5 (下限)
      want: 1.5,
    },
  ])('$name', ({ at, want }) => {
    const { controller } = started();
    controller.stall(10_100);
    arriveEvery(controller, 10_500, at, 500);

    expect(controller.target).toBeCloseTo(want);
  });

  test('空きで上げた目標量は、空きを覚える長さがたって忘れると縮む', () => {
    const { controller } = started();
    controller.arrive(12_000, 0);
    controller.stall(12_100);
    expect(controller.target).toBeCloseTo(2.75);

    // 空きを忘れるまでは、覆う量より下にしない。
    arriveEvery(controller, 12_500, 12_000 + params.gapMemoryMs - 500, 500);
    expect(controller.target).toBeCloseTo(2.75);
    // 忘れたら、無事が続いているので縮む。
    arriveEvery(controller, 12_000 + params.gapMemoryMs, 12_000 + params.gapMemoryMs + 60_000, 500);
    expect(controller.target).toBe(params.minTarget);
    expect(controller.floor).toBe(params.minTarget);
  });

  test('下限は、止まってからも、動かしてからも無事が続いたら一段ずつ下げる', () => {
    const { controller, onFloorChange } = started();
    // 2 回の説明がつかない詰まりで、下限を 1.5、2.5 にする。
    controller.stall(10_100);
    controller.stall(10_200);
    expect(controller.floor).toBe(2.5);
    expect(onFloorChange).toHaveBeenLastCalledWith(2.5);

    const decay = params.floorDecayAfterMs;
    arriveEvery(controller, 10_500, 10_200 + decay - 500, 500);
    expect(controller.floor).toBe(2.5);
    arriveEvery(controller, 10_200 + decay, 10_200 + decay + 1_000, 500);
    expect(controller.floor).toBe(2);
    // 下げてからも同じだけ無事が続くまでは、次を下げない。
    arriveEvery(controller, 10_200 + decay + 1_500, 10_200 + decay * 2 - 500, 500);
    expect(controller.floor).toBe(2);
    arriveEvery(controller, 10_200 + decay * 2, 10_200 + decay * 2 + 1_000, 500);
    expect(controller.floor).toBe(1.5);
    expect(onFloorChange).toHaveBeenLastCalledWith(1.5);
  });

  test('下限は、覆う量より下には下げない', () => {
    const { controller } = started();
    controller.stall(10_100);
    controller.stall(10_200);
    controller.stall(10_300);
    expect(controller.floor).toBe(3.5);
    // 1.6 秒おきに届き続ける (覆う量 1.6 × 1.25 + 0.25 = 2.25)。
    arriveEvery(controller, 10_500, 10_300 + params.floorDecayAfterMs * 4, 1_600);

    expect(controller.floor).toBeCloseTo(2.25);
    expect(controller.target).toBeCloseTo(2.25);
  });
});

describe('覚えた下限', () => {
  test.each([
    { name: '覚えていなければ、初期の最小量から始める', stored: null, want: 1 },
    { name: '覚えた下限から始める', stored: 2.5, want: 2.5 },
    { name: '上限より大きければ、上限に丸める', stored: 99, want: params.maxTarget },
    { name: '初期の最小量より小さければ、初期の最小量に丸める', stored: 0.2, want: 1 },
    { name: '数でなければ、覚えていないとみなす', stored: Number.NaN, want: 1 },
  ])('$name', ({ stored, want }) => {
    const controller = createLiveBufferController({ now: 0, storedFloor: stored });

    expect(controller.target).toBe(want);
    expect(controller.floor).toBe(want);
  });

  test('捨てたら、下限と目標量を初期の最小量まで詰め、覚えたものを消させる', () => {
    const { controller, onFloorChange } = started(3);
    expect(controller.status(10_000).learned).toBe(true);

    controller.forgetFloor(10_000);

    expect(controller.floor).toBe(params.minTarget);
    expect(controller.target).toBe(params.minTarget);
    expect(onFloorChange).toHaveBeenLastCalledWith(null);
    expect(controller.status(10_000).learned).toBe(false);
  });

  test.each([
    { name: '覚えた値を丸めて読む', stored: '7', want: params.maxTarget },
    { name: '覚えていなければ null', stored: null, want: null },
    { name: '数でなければ null', stored: 'abc', want: null },
  ])('loadLiveBufferFloor: $name', ({ stored, want }) => {
    expect(loadLiveBufferFloor({ getItem: () => stored })).toBe(want);
  });

  test('loadLiveBufferFloor: 読めなければ null', () => {
    expect(
      loadLiveBufferFloor({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(null);
  });

  test('saveLiveBufferFloor: 覚えた値を次に読める。null なら消す', () => {
    const store = new Map<string, string>();
    const storage = {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => void store.set(key, value),
      removeItem: (key: string) => void store.delete(key),
    };

    saveLiveBufferFloor(storage, 2.5);
    expect(loadLiveBufferFloor(storage)).toBe(2.5);
    saveLiveBufferFloor(storage, null);
    expect(loadLiveBufferFloor(storage)).toBe(null);
  });
});

describe('nextMode', () => {
  const normal = { conservative: false, speedUpAllowed: true };
  const webkit = { conservative: true, speedUpAllowed: true };
  test.each([
    {
      name: '目標量の 1.25 倍を超えたら速める',
      current: 'normal',
      compared: 1.3,
      input: normal,
      want: 'up',
    },
    {
      name: '1.25 倍までは等速のまま',
      current: 'normal',
      compared: 1.25,
      input: normal,
      want: 'normal',
    },
    {
      name: '速めている間は、1.1 倍を超える間は続ける',
      current: 'up',
      compared: 1.2,
      input: normal,
      want: 'up',
    },
    {
      name: '1.1 倍以下になったら等速に戻す',
      current: 'up',
      compared: 1.1,
      input: normal,
      want: 'normal',
    },
    {
      name: '目標量を下回ったら遅くする',
      current: 'normal',
      compared: 0.9,
      input: normal,
      want: 'down',
    },
    {
      name: '遅くしているときは、目標量に戻ったらすぐ等速に戻す',
      current: 'down',
      compared: 1,
      input: normal,
      want: 'normal',
    },
    {
      name: 'WebKit では、足りなくても遅くしない',
      current: 'normal',
      compared: 0.2,
      input: webkit,
      want: 'normal',
    },
    {
      name: 'WebKit では、目標量 + 1 秒までは速めない',
      current: 'normal',
      compared: 1.9,
      input: webkit,
      want: 'normal',
    },
    {
      name: 'WebKit では、目標量 + 1 秒を超えたら速める',
      current: 'normal',
      compared: 2.1,
      input: webkit,
      want: 'up',
    },
    {
      name: 'WebKit では、速めたら 1.05 倍以下まで続ける',
      current: 'up',
      compared: 1.06,
      input: webkit,
      want: 'up',
    },
    {
      name: '速めてはいけない間 (映像だけ遅れている間) は速めない',
      current: 'up',
      compared: 3,
      input: { conservative: false, speedUpAllowed: false },
      want: 'normal',
    },
  ] as {
    name: string;
    current: RateMode;
    compared: number;
    input: typeof normal;
    want: RateMode;
  }[])('$name', ({ current, compared, input, want }) => {
    expect(nextMode(current, compared, 1, input)).toBe(want);
  });
});

describe('decide', () => {
  const input = { conservative: false, speedUpAllowed: true };

  test('届いた直後のバッファではなく、直近の低い所に届く間隔を足して比べる', () => {
    const { controller } = started();
    // 届く直前は 0.6 秒しかない。届いた直後の 2 秒で比べると速めてしまう。
    controller.arrive(10_500, 0.6);

    expect(controller.decide(10_600, { ...input, buffer: 2 })).toEqual({ rate: 1, jump: false });
  });

  test.each([
    { name: '低い所でも溜まりすぎなら速める', low: 1.4, buffer: 1.9, want: params.speedUpRate },
    { name: '低い所が足りなければ遅くする', low: 0.2, buffer: 0.7, want: params.slowDownRate },
  ])('$name', ({ low, buffer, want }) => {
    const controller = createLiveBufferController({ now: 0, storedFloor: null });
    arriveEvery(controller, 0, 10_000, 500, low);

    expect(controller.decide(10_100, { ...input, buffer }).rate).toBe(want);
  });

  test('大きく離れたら飛ぶ。一時停止の後の追っかけでは飛ばず、追いついたらライブに戻す', () => {
    const { controller } = started();
    expect(controller.decide(10_500, { ...input, buffer: 3.6 }).jump).toBe(true);

    controller.pause();
    expect(controller.decide(15_000, { ...input, buffer: 4 })).toEqual({
      rate: params.speedUpRate,
      jump: false,
    });
    expect(controller.status(15_000).chasing).toBe(true);
    // 目標量 × 1.5 以下に戻ったら、ライブに戻す。
    controller.decide(15_100, { ...input, buffer: 1.5 });
    expect(controller.status(15_100).chasing).toBe(false);
    expect(controller.decide(15_200, { ...input, buffer: 3.6 }).jump).toBe(true);
  });

  test('読み込み直後の最初の塊は、届く直前のバッファを 0 として数えず、遅くしない', () => {
    const controller = createLiveBufferController({ now: 0, storedFloor: null });
    controller.arrive(1_000, null);
    controller.arrive(1_500, 0.6);

    // 0 を数えていれば 0 + 0.5 < 1 で遅くする。0.6 + 0.5 は目標量の 1〜1.25 倍の間なので等速。
    expect(controller.decide(1_600, { ...input, buffer: 0.6 }).rate).toBe(1);
  });

  test('作り直しや飛んだ後は、速めていた寄せ方を引き継がない', () => {
    const { controller } = started();
    expect(controller.decide(15_000, { ...input, buffer: 2 }).rate).toBe(params.speedUpRate);

    controller.selfInflicted(15_100);
    // 1.1 倍と 1.25 倍の間なので、速めている最中なら続けるが、等速からは速めない。
    expect(controller.decide(15_200, { ...input, buffer: 1.15 }).rate).toBe(1);
  });

  test('速さを変えた回数を数える', () => {
    const { controller } = started();
    // 速め、速めたまま、遅くする。
    controller.decide(15_000, { ...input, buffer: 2 });
    controller.decide(15_100, { ...input, buffer: 2 });
    controller.decide(15_200, { ...input, buffer: 0.3 });

    expect(controller.status(15_200).rateChanges).toBe(2);
  });
});

describe('prefersConservativeRate', () => {
  test.each([
    {
      name: 'Mac の Safari',
      ua: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
      touch: 0,
      want: true,
    },
    {
      name: 'Mac の User-Agent を名乗る iPad',
      ua: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
      touch: 5,
      want: true,
    },
    {
      name: 'iPhone の Chrome',
      ua: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/130.0 Mobile/15E148 Safari/604.1',
      touch: 5,
      want: true,
    },
    {
      name: 'Mac の Chrome',
      ua: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36',
      touch: 0,
      want: false,
    },
    {
      name: 'Android の Chrome',
      ua: 'Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36',
      touch: 5,
      want: false,
    },
    {
      name: 'Windows の Firefox',
      ua: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0',
      touch: 0,
      want: false,
    },
  ])('$name', ({ ua, touch, want }) => {
    expect(prefersConservativeRate(ua, touch)).toBe(want);
  });
});
