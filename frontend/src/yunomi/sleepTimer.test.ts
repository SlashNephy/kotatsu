import { describe, expect, test } from 'vitest';

import { remainingMinutes, sleepDeadline } from './sleepTimer';

describe('sleepDeadline', () => {
  test.each([
    { name: '分を選んだら、今からその分の後', choice: '30', want: 1_000 + 30 * 60_000 },
    { name: '番組の終わりなら、時刻では決めない', choice: 'programEnd', want: null },
    { name: '切ったら、止めない', choice: 'off', want: null },
  ] as const)('$name', ({ choice, want }) => {
    expect(sleepDeadline(choice, 1_000)).toBe(want);
  });
});

describe('remainingMinutes', () => {
  test.each([
    { name: '残りを分に切り上げる', deadline: 10 * 60_000, now: 61_000, want: 9 },
    { name: 'ちょうどの分は、そのまま', deadline: 10 * 60_000, now: 0, want: 10 },
    { name: '過ぎていれば 0', deadline: 1_000, now: 2_000, want: 0 },
  ])('$name', ({ deadline, now, want }) => {
    expect(remainingMinutes(deadline, now)).toBe(want);
  });
});
