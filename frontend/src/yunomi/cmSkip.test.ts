import { describe, expect, test } from 'vitest';

import { cmButtonSectionAt, cmSectionAt, mergeCmSections, shouldAutoSkip } from './cmSkip';

describe('cmButtonSectionAt', () => {
  const sections = [
    { from: 0, to: 30 },
    { from: 600, to: 690 },
  ];

  test.each([
    { name: '区間の始まりは含める', position: 600, want: sections[1] },
    { name: '区間の途中は含める', position: 650, want: sections[1] },
    { name: '区間の終わりの 1 秒の間は含めない', position: 689.5, want: null },
    { name: '本編の間は含めない', position: 300, want: null },
    { name: '録画の始まりの区間も含める', position: 0, want: sections[0] },
  ])('$name', ({ position, want }) => {
    expect(cmButtonSectionAt(sections, position)).toBe(want);
  });

  test('区間がなければ null を返す', () => {
    expect(cmButtonSectionAt([], 10)).toBeNull();
  });
});

describe('cmSectionAt', () => {
  const sections = [
    { from: 0, to: 30 },
    { from: 600, to: 690 },
  ];

  test.each([
    {
      name: '先読みの分だけ手前で、区間に入るとみなす',
      position: 599.85,
      lookahead: 0.2,
      want: sections[1],
    },
    { name: '先読みの先がまだ本編なら含めない', position: 599.7, lookahead: 0.2, want: null },
    { name: '先読みがなければ、区間の手前は含めない', position: 599.85, lookahead: 0, want: null },
    { name: '区間の途中は含める', position: 650, lookahead: 0.2, want: sections[1] },
    // 自動で飛ぶ判定では、ボタンと違い、区間の終わりの間際も CM として飛ぶ。
    { name: '区間の終わりの間際も含める', position: 689.9, lookahead: 0.2, want: sections[1] },
    { name: '区間の終わりからは含めない', position: 690, lookahead: 0.2, want: null },
  ])('$name', ({ position, lookahead, want }) => {
    expect(cmSectionAt(sections, position, lookahead)).toBe(want);
  });
});

describe('mergeCmSections', () => {
  test.each([
    {
      name: '15 秒の CM が続いた区間を、1 つにまとめる',
      sections: [
        { from: 600, to: 615 },
        { from: 615, to: 630 },
        { from: 630.4, to: 645 },
      ],
      want: [{ from: 600, to: 645 }],
    },
    {
      name: '隙間が連結の幅を超えれば、提供の読み上げのような短い本編を残す',
      sections: [
        { from: 600, to: 615 },
        { from: 620, to: 635 },
      ],
      want: [
        { from: 600, to: 615 },
        { from: 620, to: 635 },
      ],
    },
    {
      name: '並びが前後していても、始まりの順に並べてまとめる',
      sections: [
        { from: 615.5, to: 630 },
        { from: 600, to: 615 },
      ],
      want: [{ from: 600, to: 630 }],
    },
    {
      name: '重なった区間は、長いほうの終わりまで伸ばす',
      sections: [
        { from: 600, to: 640 },
        { from: 610, to: 620 },
      ],
      want: [{ from: 600, to: 640 }],
    },
    { name: '長さのない区間は捨てる', sections: [{ from: 600, to: 600 }], want: [] },
  ])('$name', ({ sections, want }) => {
    expect(mergeCmSections(sections, 1)).toEqual(want);
  });

  test('元の区間を書き換えない', () => {
    const sections = [
      { from: 600, to: 615 },
      { from: 615, to: 630 },
    ];
    mergeCmSections(sections, 1);
    expect(sections[0]).toEqual({ from: 600, to: 615 });
  });
});

describe('shouldAutoSkip', () => {
  const section = { from: 600, to: 690 };

  test.each([
    {
      name: '再生して区間に入ったら飛ばす',
      enabled: true,
      playing: true,
      lastSeek: null,
      want: true,
    },
    {
      name: '区間の外へシークした後に入ったら飛ばす',
      enabled: true,
      playing: true,
      lastSeek: 300,
      want: true,
    },
    {
      name: '自分で区間の中へシークしたら飛ばさない',
      enabled: true,
      playing: true,
      lastSeek: 620,
      want: false,
    },
    {
      name: '一時停止している間は飛ばさない',
      enabled: true,
      playing: false,
      lastSeek: null,
      want: false,
    },
    {
      name: '設定を切っていれば飛ばさない',
      enabled: false,
      playing: true,
      lastSeek: null,
      want: false,
    },
  ])('$name', ({ enabled, playing, lastSeek, want }) => {
    expect(shouldAutoSkip(section, enabled, playing, lastSeek)).toBe(want);
  });

  test('区間がなければ飛ばさない', () => {
    expect(shouldAutoSkip(null, true, true, null)).toBe(false);
  });
});
