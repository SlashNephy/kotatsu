import { describe, expect, test } from 'vitest';

import { collapseSharedColumns } from './sharedEvent';

const main = { id: 'main', programs: [{ id: 'a' }, { id: 'b' }] };

describe('collapseSharedColumns', () => {
  test.each([
    {
      name: 'その日ずっと元のサービスと同じなら、列を外す',
      sub: {
        id: 'sub',
        programs: [
          { id: 'a2', sharedFrom: { id: 'main' } },
          { id: 'b2', sharedFrom: { id: 'main' } },
        ],
      },
      wantColumns: ['main'],
      wantShared: [],
    },
    {
      name: '違う番組があれば、列を残し、同じ番組に印を付ける',
      sub: {
        id: 'sub',
        programs: [
          { id: 'a2', sharedFrom: { id: 'main' } },
          { id: 'c', sharedFrom: null },
        ],
      },
      wantColumns: ['main', 'sub'],
      wantShared: [true, false],
    },
    {
      name: '元のサービスが一覧になければ、列を残し、印も付けない',
      sub: { id: 'sub', programs: [{ id: 'a2', sharedFrom: { id: 'hidden' } }] },
      wantColumns: ['main', 'sub'],
      wantShared: [false],
    },
    {
      name: '番組のない列は残す',
      sub: { id: 'sub', programs: [] },
      wantColumns: ['main', 'sub'],
      wantShared: [],
    },
  ])('$name', ({ sub, wantColumns, wantShared }) => {
    const got = collapseSharedColumns<{
      id: string;
      programs: { id: string; sharedFrom?: { id: string } | null }[];
    }>([main, sub]);

    expect(got.map((column) => column.id)).toEqual(wantColumns);
    expect(
      got.find((column) => column.id === 'sub')?.programs.map((program) => program.shared) ?? [],
    ).toEqual(wantShared);
  });
});
