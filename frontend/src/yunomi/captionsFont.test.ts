import type { CanvasMainThreadRenderer } from 'aribb24.js';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { fontAwareRenderer } from './captions';

// FakeRenderer は、描いた字幕を覚えるだけの、aribb24.js の CanvasMainThreadRenderer の代わり。
class FakeRenderer {
  readonly rendered: string[] = [];
  render(_state: unknown, tokens: { tag: string; character?: string }[]): void {
    this.rendered.push(tokens.map((token) => token.character ?? '').join(''));
  }
  clear(): void {}
  destroy(): void {}
}

const characters = (text: string) =>
  [...text].map((character) => ({ tag: 'Character', character }));
// extended は、JIS X 0213 で足された漢字 (後の面の字)。basic は、JIS X 0208 の字だけの文。
const extended = '\u{3406}';
const basic = '今日の天気';

function stubFontLoad(): { resolve: () => void } {
  let resolve = () => {};
  const loading = new Promise<void>((done) => {
    resolve = done;
  });
  Object.defineProperty(document, 'fonts', {
    configurable: true,
    value: { load: vi.fn(() => loading.then(() => [])) },
  });

  return { resolve };
}

function newRenderer(): FakeRenderer {
  return fontAwareRenderer(
    FakeRenderer as unknown as typeof CanvasMainThreadRenderer,
    {},
  ) as unknown as FakeRenderer;
}

const render = (renderer: FakeRenderer, text: string) =>
  (renderer.render as (...args: unknown[]) => void)({}, characters(text), {});

afterEach(() => {
  // jsdom の document は fonts を持たないので、差し替えたものを外す。
  Reflect.deleteProperty(document, 'fonts');
});

describe('fontAwareRenderer', () => {
  test('基本の字だけの字幕は、待たずに描く', () => {
    stubFontLoad();
    const renderer = newRenderer();
    render(renderer, basic);
    expect(renderer.rendered).toEqual([basic]);
  });

  test('後の面の字のある字幕は、後の面を読み終えてから描く', async () => {
    const font = stubFontLoad();
    const renderer = newRenderer();
    render(renderer, `${basic}${extended}`);
    expect(renderer.rendered).toEqual([]);
    font.resolve();
    await vi.waitFor(() => expect(renderer.rendered).toEqual([`${basic}${extended}`]));
  });

  test.each([
    {
      name: '次の字幕が来たら',
      next: (renderer: FakeRenderer) => render(renderer, basic),
      want: [basic],
    },
    { name: '字幕が消えたら', next: (renderer: FakeRenderer) => renderer.clear(), want: [] },
    { name: '描画器を捨てたら', next: (renderer: FakeRenderer) => renderer.destroy(), want: [] },
  ])('後の面を待つ間に$name、古い字幕は描かない', async ({ next, want }) => {
    const font = stubFontLoad();
    const renderer = newRenderer();
    render(renderer, extended);
    next(renderer);
    font.resolve();
    // 待った後の描画が走る機会を与えてから確かめる。
    await new Promise((done) => setTimeout(done, 0));
    expect(renderer.rendered).toEqual(want);
  });
});
