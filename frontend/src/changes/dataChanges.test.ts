import { describe, expect, test } from 'vitest';

import { applyChange, changeKey, initialChangeState, type ChangeState } from './dataChanges';

// apply は、changes を順に反映する。
function apply(
  changes: { channel: 'EPG' | 'RULES' | 'RESERVATIONS'; version: number }[],
): ChangeState {
  return changes.reduce(applyChange, initialChangeState);
}

describe('applyChange', () => {
  test.each([
    {
      name: '購読を始めたときに届く今の版数は、変更として数えない',
      changes: [
        { channel: 'EPG' as const, version: 10 },
        { channel: 'RULES' as const, version: 3 },
      ],
      want: { EPG: 0, RULES: 0 },
    },
    {
      name: '版数が上がったら数える',
      changes: [
        { channel: 'EPG' as const, version: 10 },
        { channel: 'EPG' as const, version: 11 },
        { channel: 'EPG' as const, version: 12 },
      ],
      want: { EPG: 2, RULES: 0 },
    },
    {
      name: 'つなぎ直して同じ版数が届いても、数えない',
      changes: [
        { channel: 'RULES' as const, version: 3 },
        { channel: 'RULES' as const, version: 3 },
      ],
      want: { EPG: 0, RULES: 0 },
    },
    {
      name: 'つなぎ直したときに、切れている間に上がった版数が届いたら、数える',
      changes: [
        { channel: 'RULES' as const, version: 3 },
        { channel: 'RULES' as const, version: 7 },
      ],
      want: { EPG: 0, RULES: 1 },
    },
    {
      name: '古い版数は無視する',
      changes: [
        { channel: 'EPG' as const, version: 10 },
        { channel: 'EPG' as const, version: 9 },
      ],
      want: { EPG: 0, RULES: 0 },
    },
  ])('$name', ({ changes, want }) => {
    const state = apply(changes);

    expect({ EPG: changeKey(state, ['EPG']), RULES: changeKey(state, ['RULES']) }).toEqual(want);
  });
});

describe('changeKey', () => {
  test('どの種類が変わっても、値が変わる', () => {
    const base = apply([
      { channel: 'RESERVATIONS', version: 1 },
      { channel: 'RULES', version: 1 },
    ]);
    const key = changeKey(base, ['RESERVATIONS', 'RULES']);

    const reservations = applyChange(base, { channel: 'RESERVATIONS', version: 2 });
    const rules = applyChange(reservations, { channel: 'RULES', version: 2 });

    expect(changeKey(reservations, ['RESERVATIONS', 'RULES'])).not.toBe(key);
    expect(changeKey(rules, ['RESERVATIONS', 'RULES'])).not.toBe(
      changeKey(reservations, ['RESERVATIONS', 'RULES']),
    );
    // 見ていない種類が変わっても、値は変わらない。
    expect(changeKey(rules, ['RESERVATIONS'])).toBe(changeKey(reservations, ['RESERVATIONS']));
  });
});
