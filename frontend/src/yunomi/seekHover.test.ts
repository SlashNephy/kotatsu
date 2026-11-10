import { describe, expect, test } from 'vitest';

import { bandAt, bubbleLeft, hoverSeconds, partAt } from './seekHover';

describe('hoverSeconds', () => {
  test.each([
    { name: '真ん中', x: 150, want: 300 },
    { name: '左端', x: 50, want: 0 },
    { name: '左の外は 0 秒', x: 10, want: 0 },
    { name: '右の外は最後', x: 400, want: 600 },
  ])('$name', ({ x, want }) => {
    expect(hoverSeconds(x, 50, 200, 600)).toBe(want);
  });

  test('幅がなければ 0 秒', () => {
    expect(hoverSeconds(100, 50, 0, 600)).toBe(0);
  });
});

describe('bubbleLeft', () => {
  test.each([
    { name: 'そのままの位置', offset: 300, width: 800, want: 300 },
    { name: '左端に寄りすぎたら、はみ出さない位置まで戻す', offset: 10, width: 800, want: 80 },
    { name: '右端に寄りすぎたら、はみ出さない位置まで戻す', offset: 790, width: 800, want: 720 },
    { name: '吹き出しより狭いバーでは真ん中', offset: 10, width: 100, want: 50 },
  ])('$name', ({ offset, width, want }) => {
    expect(bubbleLeft(offset, width, 160)).toBe(want);
  });
});

describe('bandAt', () => {
  const bands = [
    { from: 100, to: 100, label: '途切れ' },
    { from: 200, to: 260, label: 'CM' },
  ];

  test.each([
    { name: '印のちょうどの位置', seconds: 100, want: '途切れ' },
    { name: '印の前後の許す範囲の内', seconds: 104, want: '途切れ' },
    { name: '印から離れた位置', seconds: 110, want: null },
    { name: '区間の中', seconds: 230, want: 'CM' },
    { name: '区間の外は、近くても当たらない', seconds: 197, want: null },
  ])('$name', ({ seconds, want }) => {
    expect(bandAt(bands, seconds, 5)?.label ?? null).toBe(want);
  });
});

describe('partAt', () => {
  const part = (from: number) => ({ from, to: from, label: 'A', kind: 'part' as const });
  const cm = { from: 90, to: 130, label: 'CM', kind: 'cm' as const };

  test.each([
    { name: '許容の範囲の印を返す', bands: [part(100)], seconds: 104, want: 100 },
    { name: '範囲の外なら null', bands: [part(100)], seconds: 106, want: null },
    { name: '近いほうの印を返す', bands: [part(100), part(106)], seconds: 104, want: 106 },
    { name: 'CM の区間の中でも印を返す', bands: [cm, part(100)], seconds: 101, want: 100 },
    {
      name: '区切りでない印は返さない',
      bands: [{ from: 100, to: 100, label: 'gap' }],
      seconds: 100,
      want: null,
    },
  ])('$name', ({ bands, seconds, want }) => {
    expect(partAt(bands, seconds, 5)?.from ?? null).toBe(want);
  });
});
