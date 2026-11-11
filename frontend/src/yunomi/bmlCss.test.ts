import { createRequire } from 'node:module';

import { describe, expect, test } from 'vitest';

// web-bml は、BML の文書の CSS を css パッケージで読み直してから描く。css は pnpm の override で @adobe/css-tools に
// 差し替え、宣言の前の ; を読み飛ばすパッチを当てている (pnpm-workspace.yaml、#1471)。web-bml の位置から解決し、
// web-bml が実際に読むパーサーを確かめる。
type CSSParser = {
  parse: (css: string) => unknown;
  stringify: (ast: unknown, options?: { compress?: boolean }) => string;
};
const css = createRequire(import.meta.resolve('web-bml'))('css') as CSSParser;

describe('web-bml が読む CSS のパーサー', () => {
  test.each([
    {
      name: '宣言の前の ;',
      input: 'p{;color-index:7;;font-size:20px}',
      want: 'p{color-index:7;font-size:20px;}',
    },
    {
      name: '空白を挟んで続く ;',
      input: 'p {\n  ;;  color-index: 7;\n  nav-index: 0 ;\n}',
      want: 'p{color-index:7;nav-index:0;}',
    },
    {
      name: 'BML の独自のプロパティ',
      input:
        'body{background-color-index:8;grayscale-color-index:"30 0";used-key-list:basic data-button}',
      want: 'body{background-color-index:8;grayscale-color-index:"30 0";used-key-list:basic data-button;}',
    },
  ])('$name を読める', ({ input, want }) => {
    expect(css.stringify(css.parse(input), { compress: true })).toBe(want);
  });
});
