import { beforeEach, describe, expect, test, vi } from 'vitest';

import { createInterpolator } from './interpolator';
import { createYadifInterpolation } from './yadifInterpolation';

// 補間の処理は WebGL2 が要るので、作る口だけを差し替え、yadif との受け渡しを確かめる。
vi.mock('./interpolator', () => ({ createInterpolator: vi.fn() }));

const gl = {} as WebGL2RenderingContext;
const texture = {} as WebGLTexture;

function fakeInterpolator() {
  return { estimate: vi.fn(), compose: vi.fn(), destroy: vi.fn() };
}

describe('createYadifInterpolation', () => {
  beforeEach(() => {
    vi.mocked(createInterpolator).mockReset();
  });

  test.each([
    { cadence: 'film', want: true },
    { cadence: 'frame', want: true },
    // フィールドごとの 59.94p は、放送そのもののなめらかさなので補間しない。
    { cadence: 'field', want: false },
  ] as const)('$cadence を補間するか: $want', ({ cadence, want }) => {
    expect(createYadifInterpolation().wants(cadence)).toBe(want);
  });

  test('最初の組で補間の処理を作り、同じコンテキストでは作り直さない', () => {
    const interpolator = fakeInterpolator();
    vi.mocked(createInterpolator).mockReturnValue(interpolator);
    const yadif = createYadifInterpolation();
    yadif.pair(gl, texture, texture, 1440, 1080);
    yadif.pair(gl, texture, texture, 1440, 1080);
    yadif.draw(gl, 0.5, 1440, 1080);

    expect(createInterpolator).toHaveBeenCalledTimes(1);
    expect(interpolator.estimate).toHaveBeenCalledTimes(2);
    expect(interpolator.compose).toHaveBeenCalledWith(0.5, null, 1440, 1080);
  });

  test('コンテキストが変われば、前の処理を片付けて作り直す', () => {
    const first = fakeInterpolator();
    const second = fakeInterpolator();
    vi.mocked(createInterpolator).mockReturnValueOnce(first).mockReturnValueOnce(second);
    const yadif = createYadifInterpolation();
    yadif.pair(gl, texture, texture, 1440, 1080);
    yadif.pair({} as WebGL2RenderingContext, texture, texture, 1440, 1080);

    expect(first.destroy).toHaveBeenCalledTimes(1);
    expect(second.estimate).toHaveBeenCalledTimes(1);
  });

  test.each([
    {
      name: '作れなければ',
      setup: () => {
        vi.mocked(createInterpolator).mockImplementation(() => {
          throw new Error('failed to compile a shader');
        });
      },
      act: (yadif: ReturnType<typeof createYadifInterpolation>) =>
        yadif.pair(gl, texture, texture, 1440, 1080),
    },
    {
      name: '描くときに失敗すれば',
      setup: () => {
        const interpolator = fakeInterpolator();
        interpolator.compose.mockImplementation(() => {
          throw new Error('context lost');
        });
        vi.mocked(createInterpolator).mockReturnValue(interpolator);
      },
      act: (yadif: ReturnType<typeof createYadifInterpolation>) => {
        yadif.pair(gl, texture, texture, 1440, 1080);
        yadif.draw(gl, 0.5, 1440, 1080);
      },
    },
  ])('$name、投げずに知らせ、以後は補間しない', ({ setup, act }) => {
    setup();
    const onUnavailable = vi.fn();
    const yadif = createYadifInterpolation({ onUnavailable });

    // yadif の描画の中で投げると、yadif のループが止まる。
    expect(() => act(yadif)).not.toThrow();
    expect(onUnavailable).toHaveBeenCalledTimes(1);
    expect(yadif.wants('film')).toBe(false);
    expect(yadif.wants('frame')).toBe(false);
  });
});
