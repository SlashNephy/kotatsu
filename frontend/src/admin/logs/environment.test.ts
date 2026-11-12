import { describe, expect, test } from 'vitest';

import { environmentText, otherRoles } from './environment';

const kotatsu = {
  kind: 'KOTATSU',
  version: 'v2026.11.0',
  os: 'linux',
  arch: 'amd64',
  runtime: 'go1.27.3',
};
const tuner = {
  kind: 'TUNER_SERVER',
  version: '5.3.2',
  os: 'linux',
  arch: 'amd64',
  runtime: 'mahiron',
};

describe('otherRoles', () => {
  test.each([
    { roles: ['tenban', 'neko', 'mikan', 'nekonote', 'kyusu'], want: [] },
    { roles: ['tenban'], want: ['neko', 'mikan', 'nekonote', 'kyusu'] },
  ])('$roles', ({ roles, want }) => {
    expect(otherRoles(roles)).toEqual(want);
  });
});

describe('environmentText', () => {
  test('all の構成', () => {
    expect(
      environmentText({
        sources: [kotatsu, tuner],
        roles: ['tenban', 'neko', 'mikan', 'nekonote', 'kyusu'],
        tuners: [
          { name: 'PX-Q3PE', types: ['GR'] },
          { name: 'PX-Q3PE', types: ['GR'] },
          { name: 'PX-Q3PE', types: ['BS', 'CS'] },
        ],
        userAgent: 'Mozilla/5.0',
      }),
    ).toBe(
      [
        '- Kotatsu: v2026.11.0, linux/amd64, go1.27.3',
        '- 構成: all',
        '- チューナーサーバー: mahiron, 5.3.2, linux/amd64',
        '- チューナー: PX-Q3PE (GR) × 2; PX-Q3PE (BS, CS)',
        '- ブラウザ: Mozilla/5.0',
      ].join('\n'),
    );
  });

  test('ロールを分けた構成で、版の分からないチューナーサーバー', () => {
    expect(
      environmentText({
        sources: [kotatsu, { ...tuner, version: '', os: '', arch: '', runtime: '' }],
        roles: ['tenban'],
        tuners: [],
        userAgent: 'UA',
      }),
    ).toBe(
      [
        '- Kotatsu: v2026.11.0, linux/amd64, go1.27.3',
        '- 構成: split (この報告は tenban のログだけ)',
        '- チューナーサーバー: unknown',
        '- ブラウザ: UA',
      ].join('\n'),
    );
  });
});
