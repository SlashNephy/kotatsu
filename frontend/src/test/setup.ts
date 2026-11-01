// jsdom にない、Mantine が使うブラウザの API を補う (Mantine の Vitest の手順)。

// matchMedia は、どのメディアクエリにも一致しない (OS のテーマはライト) として振る舞う。
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string): MediaQueryList => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }),
});

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
window.ResizeObserver = ResizeObserverStub;

// 画面の言語はブラウザの言語の設定で選ぶ (#831)。jsdom は英語なので、日本語の文言を確かめるテストのために日本語にする。
Object.defineProperty(navigator, 'languages', { configurable: true, value: ['ja-JP'] });
