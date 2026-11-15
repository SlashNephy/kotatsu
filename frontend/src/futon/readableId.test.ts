import { describe, expect, it } from 'vitest';

import { toGlobalId, toRawId } from './readableId';

// GraphQL の ID は、tenban の GlobalID と同じく、`型名:ID` を base64url (埋め草なし) にしたもの。
const serviceId = 'U2VydmljZTozMjczNjAxMDI0'; // Service:3273601024
const recordingId = 'UmVjb3JkaW5nOjAxOTVmM2EyLTFiMmMtN2QzZS04ZjRhLTViNmM3ZDhlOWYwMA'; // Recording:0195f3a2-1b2c-7d3e-8f4a-5b6c7d8e9f00

describe('toRawId', () => {
  it.each([
    {
      name: 'サービスはサービス ID にする',
      type: 'Service' as const,
      id: serviceId,
      want: '3273601024',
    },
    {
      name: '録画は UUID にする',
      type: 'Recording' as const,
      id: recordingId,
      want: '0195f3a2-1b2c-7d3e-8f4a-5b6c7d8e9f00',
    },
    { name: '型が違えばそのまま', type: 'Rule' as const, id: recordingId, want: recordingId },
    { name: 'GraphQL の ID でなければそのまま', type: 'Rule' as const, id: 'new', want: 'new' },
  ])('$name', ({ type, id, want }) => {
    expect(toRawId(type, id)).toBe(want);
  });
});

describe('toGlobalId', () => {
  it.each([
    {
      name: 'サービス ID を GraphQL の ID にする',
      type: 'Service' as const,
      value: '3273601024',
      want: serviceId,
    },
    {
      name: 'UUID を GraphQL の ID にする',
      type: 'Recording' as const,
      value: '0195f3a2-1b2c-7d3e-8f4a-5b6c7d8e9f00',
      want: recordingId,
    },
    {
      name: '大文字の UUID も同じ ID にする',
      type: 'Recording' as const,
      value: '0195F3A2-1B2C-7D3E-8F4A-5B6C7D8E9F00',
      want: recordingId,
    },
    {
      name: '前の形の URL (GraphQL の ID) はそのまま通す',
      type: 'Service' as const,
      value: serviceId,
      want: serviceId,
    },
    {
      name: '素の ID の形でなければそのまま通す',
      type: 'Rule' as const,
      value: 'new',
      want: 'new',
    },
  ])('$name', ({ type, value, want }) => {
    expect(toGlobalId(type, value)).toBe(want);
  });

  it('素の ID と GraphQL の ID を行き来できる', () => {
    expect(toGlobalId('Service', toRawId('Service', serviceId))).toBe(serviceId);
    expect(toGlobalId('Recording', toRawId('Recording', recordingId))).toBe(recordingId);
  });
});
