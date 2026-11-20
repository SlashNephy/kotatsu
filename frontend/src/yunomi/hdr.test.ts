import { describe, expect, test, vi } from 'vitest';

import { loadHDROutput, setHDRDrawingBuffer } from './hdr';

describe('loadHDROutput', () => {
  test.each([
    { name: '覚えていなければ、擬似 HDR にしない', stored: null, want: false },
    { name: '入れたと覚えていれば、擬似 HDR にする', stored: 'true', want: true },
    { name: '切ったと覚えていれば、擬似 HDR にしない', stored: 'false', want: false },
  ])('$name', ({ stored, want }) => {
    expect(loadHDROutput({ getItem: () => stored })).toBe(want);
  });

  test('読めなければ、擬似 HDR にしない', () => {
    expect(
      loadHDROutput({
        getItem: () => {
          throw new Error('SecurityError');
        },
      }),
    ).toBe(false);
  });
});

const RGBA8 = 0x8058;
const RGBA16F = 0x881a;

// 描画先の形式と、それを変える口だけを持つ、偽のコンテキスト。
function fakeContext({ format = RGBA8, floatColors = true } = {}) {
  const context = {
    RGBA8,
    RGBA16F,
    drawingBufferFormat: format,
    getExtension: vi.fn((name: string) =>
      name === 'EXT_color_buffer_float' && floatColors ? {} : null,
    ),
    drawingBufferStorage: vi.fn((next: number) => {
      context.drawingBufferFormat = next;
    }),
    drawingBufferToneMapping: vi.fn(),
  };

  return context;
}

describe('setHDRDrawingBuffer', () => {
  test('擬似 HDR にするときは、拡張を入れてから RGBA16F と extended にする', () => {
    const context = fakeContext();
    setHDRDrawingBuffer(context as unknown as WebGL2RenderingContext, true, 1920, 1080);

    expect(context.drawingBufferStorage).toHaveBeenCalledWith(RGBA16F, 1920, 1080);
    expect(context.drawingBufferToneMapping).toHaveBeenCalledWith({ mode: 'extended' });
  });

  test('SDR に戻すときは、RGBA8 と standard にする', () => {
    const context = fakeContext({ format: RGBA16F });
    setHDRDrawingBuffer(context as unknown as WebGL2RenderingContext, false, 1920, 1080);

    expect(context.drawingBufferStorage).toHaveBeenCalledWith(RGBA8, 1920, 1080);
    expect(context.drawingBufferToneMapping).toHaveBeenCalledWith({ mode: 'standard' });
  });

  test.each([
    { name: '既に擬似 HDR の形なら', format: RGBA16F, hdr: true },
    { name: '既に SDR の形なら', format: RGBA8, hdr: false },
  ])('$name、描画先を作り直さない (中身が消えるので)', ({ format, hdr }) => {
    const context = fakeContext({ format });
    setHDRDrawingBuffer(context as unknown as WebGL2RenderingContext, hdr, 1920, 1080);

    expect(context.drawingBufferStorage).not.toHaveBeenCalled();
    expect(context.drawingBufferToneMapping).not.toHaveBeenCalled();
  });

  test('描画先の形式を変えられなければ (alpha のないコンテキストなど)、トーンマッピングを変えずに投げる', () => {
    const context = fakeContext();
    context.drawingBufferStorage.mockImplementation(() => {});

    expect(() =>
      setHDRDrawingBuffer(context as unknown as WebGL2RenderingContext, true, 1920, 1080),
    ).toThrow();
    expect(context.drawingBufferToneMapping).not.toHaveBeenCalled();
  });

  test('浮動小数点の描画先を作れなければ、描画先を変えずに投げる', () => {
    const context = fakeContext({ floatColors: false });

    expect(() =>
      setHDRDrawingBuffer(context as unknown as WebGL2RenderingContext, true, 1920, 1080),
    ).toThrow();
    expect(context.drawingBufferStorage).not.toHaveBeenCalled();
  });
});
