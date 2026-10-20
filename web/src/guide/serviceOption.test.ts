import { describe, expect, test } from 'vitest';

import { serviceGroups } from './serviceOption';

const service = (id: string, type: string) => ({
  id,
  name: `局 ${id}`,
  channelType: { displayName: type },
});

describe('serviceGroups', () => {
  test('放送波ごとに、最初に現れた順でまとめ、中のサービスは渡した順のままにする', () => {
    const services = [
      service('1', '地上波'),
      service('2', 'BS'),
      service('3', '地上波'),
      service('4', 'CS'),
      service('5', 'BS'),
    ];

    expect(serviceGroups(services, (s) => `${s.name}!`)).toEqual([
      {
        group: '地上波',
        items: [
          { value: '1', label: '局 1!' },
          { value: '3', label: '局 3!' },
        ],
      },
      {
        group: 'BS',
        items: [
          { value: '2', label: '局 2!' },
          { value: '5', label: '局 5!' },
        ],
      },
      { group: 'CS', items: [{ value: '4', label: '局 4!' }] },
    ]);
  });
});
