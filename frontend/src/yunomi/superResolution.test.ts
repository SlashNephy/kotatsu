import { describe, expect, test, vi } from 'vitest';

import {
  createYadifUpscaler,
  loadSuperResolution,
  loadSuperResolutionMethod,
  loadSuperResolutionStrength,
  upscaleSize,
  type SuperResolutionStrength,
} from './superResolution';
import type { Anime4K } from './anime4k';
import type { HDRCopy } from './hdr';
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

describe('loadSuperResolutionMethod', () => {
  test.each([
    { name: '覚えていなければ FSR', stored: null, want: 'fsr' },
    { name: '選んだ方式を覚えていればその方式', stored: 'anime4k', want: 'anime4k' },
    { name: '知らない値なら FSR', stored: 'rife', want: 'fsr' },
  ])('$name', ({ stored, want }) => {
    expect(loadSuperResolutionMethod({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ FSR', () => {
    expect(
      loadSuperResolutionMethod({
        getItem: () => {
          throw new Error('denied');
        },
      }),
    ).toBe('fsr');
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

// GPU の上限を読む口だけを持つ、モックのコンテキスト。
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

const doubledTexture = {} as WebGLTexture;

function fakeAnime4K() {
  return { upscale: vi.fn<Anime4K['upscale']>(() => doubledTexture), destroy: vi.fn() };
}

function fakeCopy() {
  return { draw: vi.fn<HDRCopy['draw']>(), destroy: vi.fn() };
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
    expect(made.draw).toHaveBeenLastCalledWith(texture, true, 1440, 1080, 3840, 2160, 0.2, false);
    expect(upscaler.output()).toEqual({ width: 3840, height: 2160 });
  });

  test('鮮鋭化は、描くたびに読んだ強さの段数で弱める', () => {
    const made = fakeUpscaler();
    let strength: SuperResolutionStrength = 1;
    const upscaler = createYadifUpscaler({ create: () => made, strength: () => strength });
    const gl = fakeContext();

    upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160);
    expect(made.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, 3840, 2160, 2, false);
    strength = 4;
    upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160);
    expect(made.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, 3840, 2160, 0, false);
  });

  test('映す大きさは、切り抜きの拡大率を掛けて覚え、元の大きさ以下で拡大しないときの理由に使う', () => {
    let zoom = { x: 1, y: 1 };
    const upscaler = createYadifUpscaler({ zoom: () => zoom, create: fakeUpscaler });
    const gl = fakeContext();

    expect(upscaler.shown()).toBeNull();
    expect(upscaler.size(gl, 1440, 1080, 1422, 800)).toBeNull();
    expect(upscaler.shown()).toEqual({ width: 1422, height: 800 });
    zoom = { x: 1.5, y: 1.5 };
    expect(upscaler.size(gl, 1440, 1080, 1422, 800)).not.toBeNull();
    expect(upscaler.shown()).toEqual({ width: 2133, height: 1200 });
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

  test('拡大を切っていても、擬似 HDR で描くなら、元の大きさのまま描かせ、拡大しない理由は出さない', () => {
    const upscaler = createYadifUpscaler({
      upscale: () => false,
      hdr: () => true,
      create: fakeUpscaler,
    });

    expect(upscaler.size(fakeContext(), 1440, 1080, 3840, 2160)).toEqual({
      width: 1440,
      height: 1080,
    });
    expect(upscaler.shown()).toBeNull();
  });

  test('拡大も擬似 HDR も切っていれば、yadif にそのまま写させる', () => {
    const upscaler = createYadifUpscaler({ upscale: () => false, create: fakeUpscaler });

    expect(upscaler.size(fakeContext(), 1440, 1080, 3840, 2160)).toBeNull();
  });

  test('擬似 HDR では、描画先を浮動小数点にしてから、明るい所を伸ばして描く', () => {
    const copy = fakeCopy();
    const made = fakeUpscaler();
    const setDrawingBuffer = vi.fn();
    const upscaler = createYadifUpscaler({
      hdr: () => true,
      create: () => made,
      createCopy: () => copy,
      setDrawingBuffer,
    });
    const gl = fakeContext();

    expect(upscaler.draw(gl, texture, true, 1440, 1080, 1440, 1080)).toBe(true);
    expect(setDrawingBuffer).toHaveBeenLastCalledWith(gl, true, 1440, 1080);
    expect(copy.draw).toHaveBeenLastCalledWith(texture, true, 1440, 1080, true);
    expect(upscaler.output()).toBeNull();
    expect(upscaler.hdr()).toBe(true);

    expect(upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160)).toBe(true);
    expect(setDrawingBuffer).toHaveBeenLastCalledWith(gl, true, 3840, 2160);
    expect(made.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, 3840, 2160, 0.2, true);
  });

  test('擬似 HDR を切れば、描画先を SDR に戻し、伸ばさずに描く。一度も入れていなければ描画先に触れない', () => {
    let hdr = false;
    const copy = fakeCopy();
    const setDrawingBuffer = vi.fn();
    const upscaler = createYadifUpscaler({
      hdr: () => hdr,
      create: fakeUpscaler,
      createCopy: () => copy,
      setDrawingBuffer,
    });
    const gl = fakeContext();

    upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160);
    expect(setDrawingBuffer).not.toHaveBeenCalled();
    hdr = true;
    upscaler.draw(gl, texture, false, 1440, 1080, 1440, 1080);
    hdr = false;
    upscaler.draw(gl, texture, false, 1440, 1080, 1440, 1080);
    expect(setDrawingBuffer).toHaveBeenLastCalledWith(gl, false, 1440, 1080);
    expect(copy.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, false);
    expect(upscaler.hdr()).toBe(false);
  });

  test('拡大を切ったまま擬似 HDR を切っても、一度は元の大きさで描かせ、SDR に戻して描き直させる', () => {
    let hdr = true;
    const copy = fakeCopy();
    const setDrawingBuffer = vi.fn();
    const upscaler = createYadifUpscaler({
      upscale: () => false,
      hdr: () => hdr,
      create: fakeUpscaler,
      createCopy: () => copy,
      setDrawingBuffer,
    });
    const gl = fakeContext();
    upscaler.draw(gl, texture, false, 1440, 1080, 1440, 1080);
    hdr = false;

    expect(upscaler.size(gl, 1440, 1080, 1920, 1080)).toEqual({ width: 1440, height: 1080 });
    upscaler.draw(gl, texture, false, 1440, 1080, 1440, 1080);
    expect(setDrawingBuffer).toHaveBeenLastCalledWith(gl, false, 1440, 1080);
    expect(copy.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, false);
    expect(upscaler.hdr()).toBe(false);
    // SDR に戻した後は、yadif にそのままコピーさせる。
    expect(upscaler.size(gl, 1440, 1080, 1920, 1080)).toBeNull();
  });

  test('描画先を浮動小数点にできなければ、知らせて SDR で描き、以後は擬似 HDR にしない', () => {
    const copy = fakeCopy();
    const onHDRUnavailable = vi.fn();
    const setDrawingBuffer = vi.fn((_gl: WebGL2RenderingContext, hdr: boolean) => {
      if (hdr) {
        throw new Error('the drawing buffer cannot hold floating point colors');
      }
    });
    const upscaler = createYadifUpscaler({
      hdr: () => true,
      create: fakeUpscaler,
      createCopy: () => copy,
      setDrawingBuffer,
      onHDRUnavailable,
    });
    const gl = fakeContext();

    expect(upscaler.draw(gl, texture, false, 1440, 1080, 1440, 1080)).toBe(true);
    expect(onHDRUnavailable).toHaveBeenCalledTimes(1);
    expect(copy.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, false);
    expect(upscaler.hdr()).toBe(false);
    // 拡大しない大きさなら、元の大きさで描かせず、yadif にそのままコピーさせる。
    expect(upscaler.size(gl, 1440, 1080, 1422, 800)).toBeNull();
  });

  // Anime4K は 2 倍に決まっているので、縦横とも 2 倍以上に拡大するときだけ先に 2 倍にし、残りを FSR で拡大する (#1653)。
  test.each([
    {
      name: '縦横とも 2 倍以上なら、Anime4K で 2 倍にしてから、残りを FSR で拡大する',
      method: 'anime4k' as const,
      output: { width: 3840, height: 2160 },
      want: { texture: doubledTexture, width: 2880, height: 2160, anime4k: true },
    },
    {
      name: '横だけを拡大するなら、Anime4K を使わない',
      method: 'anime4k' as const,
      output: { width: 1920, height: 1080 },
      want: { texture, width: 1440, height: 1080, anime4k: false },
    },
    {
      name: 'FSR を選んでいれば、Anime4K を使わない',
      method: 'fsr' as const,
      output: { width: 3840, height: 2160 },
      want: { texture, width: 1440, height: 1080, anime4k: false },
    },
  ])('$name', ({ method, output, want }) => {
    const made = fakeUpscaler();
    const anime4k = fakeAnime4K();
    const upscaler = createYadifUpscaler({
      create: () => made,
      createAnime4KUpscaler: () => anime4k,
      method: () => method,
    });

    upscaler.draw(fakeContext(), texture, true, 1440, 1080, output.width, output.height);
    expect(anime4k.upscale.mock.calls).toEqual(want.anime4k ? [[texture, true, 1440, 1080]] : []);
    expect(made.draw).toHaveBeenLastCalledWith(
      want.texture,
      true,
      want.width,
      want.height,
      output.width,
      output.height,
      0.2,
      false,
    );
    expect(upscaler.anime4k()).toBe(want.anime4k);
  });

  test('Anime4K が途中で失敗すれば、片付けて知らせ、そのコマから FSR だけで拡大する', () => {
    const made = fakeUpscaler();
    const anime4k = fakeAnime4K();
    anime4k.upscale.mockImplementation(() => {
      throw new Error('context lost');
    });
    const onAnime4KUnavailable = vi.fn();
    const upscaler = createYadifUpscaler({
      create: () => made,
      createAnime4KUpscaler: () => anime4k,
      method: () => 'anime4k',
      onAnime4KUnavailable,
    });
    const gl = fakeContext();

    expect(upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160)).toBe(true);
    expect(upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160)).toBe(true);
    expect(anime4k.upscale).toHaveBeenCalledTimes(1);
    expect(anime4k.destroy).toHaveBeenCalledTimes(1);
    expect(onAnime4KUnavailable).toHaveBeenCalledTimes(1);
    expect(made.draw).toHaveBeenCalledTimes(2);
    expect(made.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, 3840, 2160, 0.2, false);
  });

  test('Anime4K を作れなければ、知らせて FSR だけで拡大し、以後は作り直さない', () => {
    const made = fakeUpscaler();
    const createAnime4KUpscaler = vi.fn((): Anime4K => {
      throw new Error('no EXT_color_buffer_float');
    });
    const onAnime4KUnavailable = vi.fn();
    const upscaler = createYadifUpscaler({
      create: () => made,
      createAnime4KUpscaler,
      method: () => 'anime4k',
      onAnime4KUnavailable,
    });
    const gl = fakeContext();

    expect(upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160)).toBe(true);
    expect(upscaler.draw(gl, texture, false, 1440, 1080, 3840, 2160)).toBe(true);
    expect(createAnime4KUpscaler).toHaveBeenCalledTimes(1);
    expect(onAnime4KUnavailable).toHaveBeenCalledTimes(1);
    expect(made.draw).toHaveBeenLastCalledWith(texture, false, 1440, 1080, 3840, 2160, 0.2, false);
    expect(upscaler.anime4k()).toBe(false);
  });
});
