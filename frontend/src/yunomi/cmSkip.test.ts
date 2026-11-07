import { describe, expect, test } from 'vitest';

import { cmSectionAt } from './cmSkip';

describe('cmSectionAt', () => {
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
    expect(cmSectionAt(sections, position)).toBe(want);
  });

  test('区間がなければ null を返す', () => {
    expect(cmSectionAt([], 10)).toBeNull();
  });
});
