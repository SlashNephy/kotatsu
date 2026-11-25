import { describe, expect, test } from 'vitest';

import { DecodeJudge, type JudgeParams } from './decodeJudge';

// 1 コマ (29.97 fps) の長さ。
const frameMs = 1_000 / 29.97;

const params: Partial<JudgeParams> = {
  windowMs: 1_000,
  minFrames: 20,
  ignoreMs: 2_000,
  skipInitialMs: 5_000,
  relapseMs: 5_000,
  skipMaxMs: 12_000,
  audioGapMs: 10_000,
  audioFailures: 3,
};

type Window = { decodeMs?: number; frames?: number; dropped?: number; lagMs?: number };

// feed は、1 ウィンドウ分のデコードの様子を渡し、ウィンドウの終わりの tick の結果を返す。音と映像は届いている。
function feed(judge: DecodeJudge, now: number, window: Window) {
  const { decodeMs = 10, frames = 30, dropped = 0, lagMs = 0 } = window;
  judge.onVideo(now);
  judge.onAudio(now);
  for (let i = 0; i < frames; i++) {
    judge.onDecode(decodeMs);
    judge.onPicture();
  }
  judge.onDropped(dropped);
  judge.onLag(lagMs);

  return judge.tick(now, frameMs);
}

// started は、読み始めて、数えない時間が過ぎ、最初のウィンドウが始まった判定を返す。
function started(): { judge: DecodeJudge; now: number } {
  const judge = new DecodeJudge(params);
  judge.start(0);
  judge.tick(2_000, frameMs);

  return { judge, now: 2_000 };
}

describe('DecodeJudge', () => {
  test.each([
    { name: '間に合っている', window: {}, fallback: false },
    {
      name: 'デコード時間の分位が持ち時間の割合を超える',
      window: { decodeMs: 30 },
      fallback: true,
    },
    { name: '映像が遅れた', window: { lagMs: 600 }, fallback: true },
    { name: '破棄したコマが枚数と割合の両方を超える', window: { dropped: 12 }, fallback: true },
    { name: '破棄したコマが割合に届かない', window: { frames: 200, dropped: 12 }, fallback: false },
    {
      name: 'コマが少ないウィンドウは数えない',
      window: { decodeMs: 30, frames: 10 },
      fallback: false,
    },
  ])('悪いウィンドウが 3 回続いたらほかの経路へ移す: $name', ({ window, fallback }) => {
    let { judge, now } = started();
    const verdicts = [];
    for (let i = 0; i < 3; i++) {
      now += 1_000;
      verdicts.push(...feed(judge, now, window));
    }

    expect(verdicts.some((verdict) => verdict.type === 'fallback')).toBe(fallback);
  });

  test('1 回の重さでは戻さない (悪いウィンドウのあとに無事なウィンドウが挟まると数え直す)', () => {
    let { judge, now } = started();
    const verdicts = [];
    for (const window of [
      { decodeMs: 30 },
      { decodeMs: 30 },
      {},
      { decodeMs: 30 },
      { decodeMs: 30 },
    ]) {
      now += 1_000;
      verdicts.push(...feed(judge, now, window));
    }

    expect(verdicts.filter((verdict) => verdict.type === 'fallback')).toEqual([]);
  });

  test('読み始めてから数えない時間のウィンドウは数えない', () => {
    const judge = new DecodeJudge(params);
    judge.start(0);
    judge.tick(0, frameMs);
    const verdicts = [];
    for (let now = 1_000; now <= 2_000; now += 1_000) {
      verdicts.push(...feed(judge, now, { decodeMs: 30 }));
    }

    expect(verdicts).toEqual([]);
  });

  test('タブが戻った直後は数えないが、それまでの悪いウィンドウの数は残す', () => {
    let { judge, now } = started();
    for (let i = 0; i < 2; i++) {
      now += 1_000;
      feed(judge, now, { decodeMs: 30 });
    }
    judge.resume(now);
    // 戻ってから数えない時間 (2 秒) のウィンドウは数えない。
    now += 1_000;
    expect(feed(judge, now, { decodeMs: 30 })).toEqual([]);
    now += 1_000;
    feed(judge, now, { decodeMs: 30 });
    now += 1_000;

    expect(feed(judge, now, { decodeMs: 30 })).toEqual([{ type: 'fallback', reason: 'slow' }]);
  });

  test('データが来ない間と、来てからしばらくは数えない', () => {
    let { judge, now } = started();
    judge.starve();
    now += 1_000;
    judge.onPicture();
    expect(judge.tick(now, frameMs)).toEqual([]);
    // データが来た。ここから数えない時間 (2 秒) の間のウィンドウは、悪くても数えない。
    for (let i = 0; i < 2; i++) {
      now += 1_000;
      expect(feed(judge, now, { lagMs: 600 })).toEqual([]);
    }
  });

  test('少し遅れたら B フレームを省き、決めた長さが過ぎたら戻す', () => {
    let { judge, now } = started();
    now += 1_000;
    expect(feed(judge, now, { dropped: 5, frames: 100 })).toEqual([{ type: 'skipB', on: true }]);
    expect(judge.skippingB).toBe(true);
    // 5 秒 (skipInitialMs) 省いたら戻す。
    now += 5_000;
    expect(feed(judge, now, {})).toEqual([{ type: 'skipB', on: false }]);
    expect(judge.skippingB).toBe(false);
  });

  test('戻してすぐにまた遅れたら、省く長さを倍にし、上限で止める', () => {
    let { judge, now } = started();
    const durations: number[] = [];
    for (let i = 0; i < 3; i++) {
      now += 1_000;
      feed(judge, now, { lagMs: 250 });
      const from = now;
      while (judge.skippingB) {
        now += 1_000;
        feed(judge, now, {});
      }
      durations.push(now - from);
    }

    // 5 秒、10 秒、20 秒の代わりに上限の 12 秒。
    expect(durations).toEqual([5_000, 10_000, 12_000]);
  });

  test('戻してしばらく遅れなければ、省く長さを初めに戻す', () => {
    let { judge, now } = started();
    now += 1_000;
    feed(judge, now, { lagMs: 250 });
    now += 5_000;
    feed(judge, now, {});
    // 戻してから 5 秒 (relapseMs) より後に遅れた。
    for (let i = 0; i < 6; i++) {
      now += 1_000;
      feed(judge, now, {});
    }
    now += 1_000;
    feed(judge, now, { lagMs: 250 });
    const from = now;
    while (judge.skippingB) {
      now += 1_000;
      feed(judge, now, {});
    }

    expect(now - from).toBe(5_000);
  });

  test('B フレームを省いている間は、デコード時間の許容を広げる', () => {
    let { judge, now } = started();
    now += 1_000;
    feed(judge, now, { lagMs: 250 });
    const verdicts = [];
    // 1 コマの長さ × 0.7 (約 23 ms) は超えるが、その 3 倍は超えない。
    for (let i = 0; i < 3; i++) {
      now += 1_000;
      verdicts.push(...feed(judge, now, { decodeMs: 30 }));
    }

    expect(verdicts).toEqual([]);
  });

  test('映像は届いているのに音が来なければ、ほかの経路へ移す', () => {
    let { judge, now } = started();
    const verdicts = [];
    while (now < 12_000) {
      now += 1_000;
      judge.onVideo(now);
      verdicts.push(...judge.tick(now, frameMs));
    }

    expect(verdicts).toEqual([{ type: 'fallback', reason: 'audio' }]);
  });

  test('AAC のデコードが続けて失敗したら戻すが、1 回の失敗では戻さない', () => {
    let { judge, now } = started();
    for (const ok of [false, true, false, false, false]) {
      judge.onAudioDecoded(ok);
    }
    now += 1_000;
    expect(feed(judge, now, {})).toEqual([]);
    judge.onAudioDecoded(false);
    now += 1_000;

    expect(feed(judge, now, {})).toEqual([{ type: 'fallback', reason: 'audio' }]);
  });

  test('一度戻したら、そのあとは何も返さない', () => {
    let { judge, now } = started();
    for (let i = 0; i < 3; i++) {
      now += 1_000;
      feed(judge, now, { decodeMs: 30 });
    }
    now += 1_000;

    expect(feed(judge, now, { decodeMs: 30 })).toEqual([]);
  });
});
