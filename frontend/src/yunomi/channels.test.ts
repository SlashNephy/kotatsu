import { describe, expect, test } from 'vitest';

import { neighborServices } from './channels';

const service = (id: string, type: string) => ({ id, channelType: { type } });
const services = [
  service('gr1', 'GR'),
  service('gr2', 'GR'),
  service('gr3', 'GR'),
  service('bs1', 'BS'),
];

describe('neighborServices', () => {
  test.each([
    { name: '真ん中', current: 'gr2', want: { previous: 'gr1', next: 'gr3' } },
    { name: '先頭では、前は末尾へ回る', current: 'gr1', want: { previous: 'gr3', next: 'gr2' } },
    { name: '末尾では、次は先頭へ回る', current: 'gr3', want: { previous: 'gr2', next: 'gr1' } },
    {
      name: '同じ種別にほかのサービスがなければ、どちらもない',
      current: 'bs1',
      want: { previous: null, next: null },
    },
    { name: '知らないサービス', current: 'x', want: { previous: null, next: null } },
  ])('$name', ({ current, want }) => {
    const got = neighborServices(services, current);
    expect({ previous: got.previous?.id ?? null, next: got.next?.id ?? null }).toEqual(want);
  });
});
