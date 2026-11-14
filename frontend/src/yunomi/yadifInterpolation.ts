import type { Cadence, PictureInterpolator } from '@mpeg2toh264/yadif';

import { createDrawPacer } from './interpolationTiming';
import { createInterpolator, type Interpolator } from './interpolator';

// yadif が解除したコマの間を補間する (#1655、設計書 7.2)。yadif は、補間するコマを 1 枚遅らせて出し、
// その間の画面の書き換えごとに、自分の WebGL2 のコンテキストとテクスチャを渡してくる。
// 補間の処理 (interpolator.ts) は、そのコンテキストのまま動かすので、GPU の外へ写さない。

// interpolatedCadences は、補間するコマの出し方。24p (プルダウンを組み直したもの) と、コマごとの 29.97p だけにする。
// フィールドごとの 59.94p は、放送そのもののなめらかさで、間を埋める画面の書き換えもほとんどない。
const interpolatedCadences: ReadonlySet<Cadence> = new Set(['film', 'frame']);

// YadifInterpolation は、yadif に渡す補間と、それを片付ける口。
export type YadifInterpolation = PictureInterpolator & { destroy(): void };

// createYadifInterpolation は、yadif に渡す補間を作る。補間の処理は、yadif が最初にコマの組を渡してきたときに、
// そのコンテキストで作る。作れないか、途中で失敗したら、onUnavailable を呼び、以後は補間しない (yadif はそのまま出す)。
// yadif の描画の中で投げると、yadif のループが止まるので、ここで受け止める。
// maxRate は、補間したコマを出す回数の上限 (1 秒あたり)。null なら、画面の書き換えごとに描く。
export function createYadifInterpolation({
  onUnavailable,
  maxRate = () => null,
}: {
  onUnavailable?: (error: unknown) => void;
  maxRate?: () => number | null;
} = {}): YadifInterpolation {
  let context: WebGL2RenderingContext | null = null;
  let interpolator: Interpolator | null = null;
  let broken = false;
  const pacer = createDrawPacer();
  // yadif は、補間する間、画面の書き換えごとに draw を呼ぶので、その間隔から書き換えの間隔を測る。
  let refreshMs = 1_000 / 60;
  let lastCallAt: number | null = null;

  const fail = (error: unknown) => {
    broken = true;
    try {
      interpolator?.destroy();
    } catch {
      // コンテキストを失っていれば、片付けられなくてもよい。
    }
    interpolator = null;
    onUnavailable?.(error);
  };

  return {
    wants(cadence) {
      return !broken && interpolatedCadences.has(cadence);
    },
    pair(gl, previous, next, width, height) {
      if (broken) {
        return;
      }
      try {
        if (context !== gl || interpolator === null) {
          interpolator?.destroy();
          interpolator = createInterpolator(gl);
          context = gl;
        }
        interpolator.estimate(previous, next, width, height);
      } catch (error) {
        fail(error);
      }
    },
    draw(_gl, t, width, height) {
      if (broken || interpolator === null) {
        return false;
      }
      const now = performance.now();
      if (lastCallAt !== null) {
        const step = now - lastCallAt;
        // 測り始めの値より速い画面なら、すぐ合わせる。間に補間しなかった時間 (コマをそのまま出した書き換え、
        // 一時停止) は、書き換えの間隔に数えない。
        if (step > 0 && step < refreshMs * 0.75) {
          refreshMs = step;
        } else if (step > 0 && step < refreshMs * 1.5) {
          refreshMs += (step - refreshMs) * 0.1;
        }
      }
      lastCallAt = now;
      if (!pacer.shouldDraw(now, maxRate(), refreshMs)) {
        return false;
      }
      try {
        interpolator.compose(t, null, width, height);

        return true;
      } catch (error) {
        fail(error);

        return false;
      }
    },
    destroy() {
      try {
        interpolator?.destroy();
      } catch {
        // コンテキストを失っていれば、片付けられなくてもよい。
      }
      interpolator = null;
      context = null;
    },
  };
}
