import { describe, expect, test } from 'vitest';

import { parseUrlText } from './useUrlText';

describe('parseUrlText', () => {
  test.each([
    { name: '文字列はそのまま', value: ' ニュース ', want: ' ニュース ' },
    { name: '数として読んだ語は、文字列に戻す', value: 2026, want: '2026' },
    { name: '真偽値として読んだ語は、文字列に戻す', value: false, want: 'false' },
    { name: 'null として読んだ語は、文字列に戻す', value: null, want: 'null' },
    { name: '空白だけの語は破棄する', value: '  ', want: undefined },
    { name: 'ないときは undefined', value: undefined, want: undefined },
    { name: '配列は破棄する', value: ['a'], want: undefined },
  ])('$name', ({ value, want }) => {
    expect(parseUrlText(value)).toBe(want);
  });
});
