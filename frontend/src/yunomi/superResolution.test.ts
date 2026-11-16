import { describe, expect, test, vi } from 'vitest';

import {
  createYadifUpscaler,
  loadSuperResolution,
  loadSuperResolutionStrength,
  upscaleSize,
  type SuperResolutionStrength,
} from './superResolution';
import type { Upscaler } from './upscaler';

describe('loadSuperResolution', () => {
  test.each([
    { name: '覚えていなければ拡大しない', stored: null, want: false },
    { name: '入れたことを覚えていれば拡大する', stored: 'true', want: true },
    { name: '切ったことを覚えていれば拡大しない', stored: 'false', want: false },
  ])('$name', ({ stored, want }) => {
    expect(loadSuperResolution({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ拡大しない', () => {
    expect(
      loadSuperResolution({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(false);
  });
});

describe('loadSuperResolutionStrength', () => {
  test.each([
    { name: '覚えていなければ標準の 3', stored: null, want: 3 },
    { name: '選んだ強さを覚えていればその強さ', stored: '1', want: 1 },
    { name: '知らない値なら標準の 3', stored: '9', want: 3 },
    { name: '数でなければ標準の 3', stored: 'strong', want: 3 },
  ])('$name', ({ stored, want }) => {
    expect(loadSuperResolutionStrength({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ標準の 3', () => {
    expect(
      loadSuperResolutionStrength({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe(3);
  });
});

const broadcast = { width: 1440, height: 1080 };
const noZoom = { x: 1, y: 1 };

describe('upscaleSize', () => {
  test.each([
    {
      name: '4K の画面いっぱいに映すなら、4K に拡大する',
      source: broadcast,
      box: { width: 3840, height: 2160 },
      zoom: noZoom,
      max: 16384,
      want: { width: 3840, height: 2160 },
    },
    {
      // 1440×1080 の放送は、横にだけ引き伸ばして 16:9 で見る。縦は元の大きさのまま、横だけ拡大する。
      name: 'フル HD の画面では、横だけを拡大する',
      source: broadcast,
      box: { width: 1920, height: 1080 },
      zoom: noZoom,
      max: 16384,
      want: { width: 1920, height: 1080 },
    },
    {
      name: '縦が元より小さく映るときは、縦を元の大きさに保つ',
      source: broadcast,
      box: { width: 1600, height: 900 },
      zoom: noZoom,
      max: 16384,
      want: { width: 1600, height: 1080 },
    },
    {
      name: '縦も横も元の大きさ以下に映すなら、拡大しない',
      source: broadcast,
      box: { width: 1280, height: 720 },
      zoom: noZoom,
      max: 16384,
      want: null,
    },
    {
      name: '切り抜きで拡大して映すなら、その分も大きく描く',
      source: broadcast,
      box: { width: 1280, height: 720 },
      zoom: { x: 1.5, y: 1.5 },
      max: 16384,
      want: { width: 1920, height: 1080 },
    },
    {
      name: '横だけに伸ばす切り抜きは、横だけに掛ける',
      source: { width: 1920, height: 1080 },
      box: { width: 1920, height: 1080 },
      zoom: { x: 1.33, y: 1 },
      max: 16384,
      want: { width: 2554, height: 1080 },
    },
    {
      name: 'GPU が描ける一辺を超えるなら、比を保って縮める',
      source: broadcast,
      box: { width: 7680, height: 4320 },
      zoom: noZoom,
      max: 4096,
      want: { width: 4096, height: 2304 },
    },
    {
      name: '描ける一辺に縮めると元の大きさ以下になるなら、拡大しない',
      source: { width: 1920, height: 1080 },
      box: { width: 7680, height: 1100 },
      zoom: noZoom,
      max: 1920,
      want: null,
    },
    {
      name: '映す大きさがまだ決まっていなければ、拡大しない',
      source: broadcast,
      box: { width: 0, height: 0 },
      zoom: noZoom,
      max: 16384,
      want: null,
    },
  ])('$name', ({ source, box, zoom, max, want }) => {
    expect(upscaleSize(source, box, zoom, max)).toEqual(want);
  });
});

// GPU の上限を読む口だけを持つ、偽のコンテキスト。
function fakeContext(maxSize = 16384): WebGL2RenderingContext {
  const MAX_TEXTURE_SIZE = 0x0d33;
  const MAX_VIEWPORT_DIMS = 0x0d3a;

  return {
    MAX_TEXTURE_SIZE,
    MAX_VIEWPORT_DIMS,
    getParameter: (name: number) =>
      name === MAX_TEXTURE_SIZE ? maxSize : new Int32Array([maxSize, maxSize]),
  } as unknown as WebGL2RenderingContext;
}

function fakeUpscaler() {
  return { draw: vi.fn<Upscaler['draw']>(), destroy: vi.fn() };
}

const texture = {} as WebGLTexture;

describe('createYadifUpscaler', () => {
  test('描く大きさは、GPU の上限と切り抜きの拡大率を見て決める', () => {
    const upscaler = createYadifUpscaler({ zoom: () => ({ x: 2, y: 2 }), create: fakeUpscaler });

    expect(upscaler.size(fakeContext(2048), 1440, 1080, 1920, 1080)).toEqual({
      width: 2048,
      height: 1152,
    });
  });

  test('最初に描くときに拡大の処理を作り、同じコンテキストでは作り直さない', () => {
    const made = fakeUpscaler();
    const create = vi.fn(() => made);
    const upscaler = createYadifUpscaler({ create });
    const gl = fakeContext();

    expect(upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160)).toBe(true);
    expect(upscaler.draw(gl, texture, true, 1440, 1080, 3840, 2160)).toBe(true);
    expect(create).toHaveBeenCalledTimes(1);
    expect(made.draw).toHaveBeenLastCalledWith(texture, true, 1440, 1080, 3840, 2160, 0.2);
    expect(upscaler.output()).toEqual({ width: 3840, height: 2160 });
  });

  test('鮮鋭化は、描くたびに読んだ強さの段数で弱める', () => {
    const made = fakeUpscaler();
    let strength: SuperResolutionStrength = 1;
    const upscaler = createYadifUpscaler({ create: () => made, strength: () => strength });
    const gl = fakeContext();

    upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160);
    expect(made.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, 3840, 2160, 2);
    strength = 4;
    upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160);
    expect(made.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, 3840, 2160, 0);
  });

  test('コンテキストを失っていれば (上限が読めなければ)、拡大しない', () => {
    const lost = {
      getParameter: () => null,
    } as unknown as WebGL2RenderingContext;
    const onUnavailable = vi.fn();
    const upscaler = createYadifUpscaler({ create: fakeUpscaler, onUnavailable });

    expect(upscaler.size(lost, 1440, 1080, 3840, 2160)).toBeNull();
    expect(onUnavailable).not.toHaveBeenCalled();
  });

  test('コンテキストが変われば、前の処理を片付けて作り直す', () => {
    const first = fakeUpscaler();
    const second = fakeUpscaler();
    const create = vi.fn().mockReturnValueOnce(first).mockReturnValueOnce(second);
    const upscaler = createYadifUpscaler({ create });
    upscaler.draw(fakeContext(), texture, false, 1440, 1080, 3840, 2160);
    upscaler.draw(fakeContext(), texture, false, 1440, 1080, 3840, 2160);

    expect(first.destroy).toHaveBeenCalledTimes(1);
    expect(second.draw).toHaveBeenCalledTimes(1);
  });

  test('拡大しない大きさになれば、拡大している大きさを null に戻す', () => {
    const upscaler = createYadifUpscaler({ create: fakeUpscaler });
    const gl = fakeContext();
    upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160);

    expect(upscaler.size(gl, 1440, 1080, 640, 360)).toBeNull();
    expect(upscaler.output()).toBeNull();
  });

  test.each([
    {
      name: '作れなければ',
      create: () => {
        throw new Error('failed to compile a shader');
      },
    },
    {
      name: '描くときに失敗すれば',
      create: () => {
        const made = fakeUpscaler();
        made.draw.mockImplementation(() => {
          throw new Error('context lost');
        });

        return made;
      },
    },
  ])('$name、投げずに知らせ、以後は拡大しない', ({ create }) => {
    const onUnavailable = vi.fn();
    const upscaler = createYadifUpscaler({ create, onUnavailable });
    const gl = fakeContext();

    // yadif の描画の中で投げると、yadif のループが止まる。false を返せば、yadif はコマをそのまま描く。
    expect(upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160)).toBe(false);
    expect(onUnavailable).toHaveBeenCalledTimes(1);
    expect(upscaler.size(gl, 1440, 1080, 3840, 2160)).toBeNull();
    expect(upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160)).toBe(false);
    expect(upscaler.output()).toBeNull();
  });
});
