import { describe, expect, test } from 'vitest';

import { PictureQueue, type QueuedPicture } from './pictureQueue';

// 29.97 fps の 1 フレームは、PTS で 3003。
const frame = 3003;

function picture(pts: number, topFieldFirst = true, interlaced = true): QueuedPicture<string> {
  return { pts, interlaced, topFieldFirst, slot: `s${pts}` };
}

function setup() {
  const released: string[] = [];
  const queue = new PictureQueue<string>((slot) => released.push(slot));

  return { queue, released };
}

describe('PictureQueue', () => {
  test('フレームの前半は 1 枚目、後半は 2 枚目のフィールドを映す', () => {
    const { queue } = setup();
    queue.push(picture(0));
    queue.push(picture(frame));

    const choices = [0, 1500, 1502, 3000].map((target) => {
      const choice = queue.choose(target);

      return choice === null ? null : [choice.picture.pts, choice.second, choice.parity];
    });

    expect(choices).toEqual([
      [0, false, 0],
      [0, false, 0],
      [0, true, 1],
      [0, true, 1],
    ]);
  });

  test('ボトムフィールドが先のフレームは、下のフィールドから映す', () => {
    const { queue } = setup();
    queue.push(picture(0, false));
    queue.push(picture(frame, false));

    expect([queue.choose(0)?.parity, queue.choose(2000)?.parity]).toEqual([1, 0]);
  });

  test('プログレッシブのフレームは、フレーム全体を映す', () => {
    const { queue } = setup();
    queue.push(picture(0, true, false));
    queue.push(picture(frame, true, false));

    expect([queue.choose(2000)?.second, queue.choose(2000)?.parity]).toEqual([false, 0]);
  });

  test('番を過ぎたフレームは映さずに捨て、フレーム落ちとして数える。映したフレームは前のフレームとして残す', () => {
    const { queue, released } = setup();
    for (let i = 0; i < 4; i++) {
      queue.push(picture(i * frame));
    }
    queue.choose(0);
    const choice = queue.choose(3 * frame);

    expect(choice?.picture.pts).toBe(3 * frame);
    expect(queue.previous?.pts).toBe(0);
    expect(queue.dropped).toBe(2);
    expect(released).toEqual([`s${frame}`, `s${2 * frame}`]);
  });

  test('まだ番の来ていないフレームは映さない', () => {
    const { queue } = setup();
    queue.push(picture(frame));

    expect(queue.choose(frame - 1)).toBeNull();
  });

  test('前より古い時刻のフレームは足さずに捨てる', () => {
    const { queue, released } = setup();
    queue.push(picture(frame));

    expect(queue.push(picture(0))).toBe(false);
    expect(released).toEqual(['s0']);
    expect(queue.length).toBe(1);
  });

  test('フレームの長さを渡せば、隣り合うフレームの時刻の差が短くても、その長さでフィールドを分ける', () => {
    const { queue } = setup();
    // 組み直した 24p のフレーム (プログレッシブ) の 0.25 フレーム後に、インタレースのフレームが続く。
    queue.push({ pts: 0, interlaced: false, topFieldFirst: true, slot: 's0' });
    queue.push({ ...picture(frame / 4), duration: frame });

    const choice = queue.choose(frame / 4 + 1000);

    expect(choice === null ? null : [choice.picture.pts, choice.second]).toEqual([
      frame / 4,
      false,
    ]);
  });

  test('following は、映しているフレームの次のフレームを返す', () => {
    const { queue } = setup();
    queue.push(picture(0));
    queue.push(picture(frame));
    queue.choose(0);

    expect(queue.following?.pts).toBe(frame);

    queue.choose(frame);

    expect(queue.following).toBeNull();
  });

  test('clear で、待っているフレームと前のフレームの slot を返す', () => {
    const { queue, released } = setup();
    queue.push(picture(0));
    queue.push(picture(frame));
    queue.push(picture(2 * frame));
    queue.choose(0);
    queue.choose(frame);
    queue.clear();

    expect(released.toSorted()).toEqual(['s0', `s${frame}`, `s${2 * frame}`].toSorted());
    expect(queue.length).toBe(0);
    expect(queue.newest).toBeNull();
  });

  test('clear で残すと言った slot は返さない (止めた後も、映していたフレームを描き直せるように)', () => {
    const { queue, released } = setup();
    queue.push(picture(0));
    queue.push(picture(frame));
    queue.push(picture(2 * frame));
    queue.choose(0);
    queue.choose(frame);
    queue.clear(`s${frame}`);

    expect(released.toSorted()).toEqual(['s0', `s${2 * frame}`].toSorted());
    expect(queue.length).toBe(0);
  });
});
