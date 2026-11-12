import { cleanup, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderWithRouter } from '../test/route';
import { SkippedDuplicateList } from './SkippedDuplicateList';

afterEach(() => {
  cleanup();
});

// program は、見送った番組か、代わりに録る放送。
function program(id: string, name: string, serviceName: string, startAt: string) {
  return {
    id,
    name,
    startAt,
    endAt: startAt.replace('T12', 'T13'),
    service: {
      id: `Service:${serviceName}`,
      name: serviceName,
      logoUrl: null,
      channelType: { id: 'ChannelType:GR', type: 'GR' },
    },
  };
}

const rules = [{ id: 'Rule:1', name: '灯台の地図' }];

describe('重複で見送りの理由 (#1498)', () => {
  test('見送った理由ごとに、録った局と日時か、代わりに録る放送を出す', async () => {
    const environment = createTestEnvironment(() => ({
      data: {
        skippedDuplicates: [
          {
            program: program('Program:1', '夜のニュース', 'こたつテスト1', '2026-11-20T12:00:00Z'),
            rules,
            reason: 'DUPLICATE_TITLE',
            lastRecordedAt: '2026-11-18T13:00:00Z',
            recorded: null,
            instead: null,
          },
          {
            program: program('Program:2', '灯台の地図 #2', 'こたつテスト2', '2026-11-20T12:00:00Z'),
            rules,
            reason: 'RECORDED_EPISODE',
            lastRecordedAt: '2026-11-15T13:00:00Z',
            recorded: {
              service: { id: 'Service:BS', name: 'こたつ BS' },
              endAt: '2026-11-15T13:00:00Z',
            },
            instead: null,
          },
          {
            program: program('Program:3', '灯台の地図 #3', 'こたつ BS', '2026-11-21T12:00:00Z'),
            rules,
            reason: 'OTHER_BROADCAST',
            lastRecordedAt: null,
            recorded: null,
            instead: {
              id: 'Program:4',
              startAt: '2026-11-20T12:00:00Z',
              service: { id: 'Service:GR', name: 'こたつテスト1' },
            },
          },
          {
            program: program('Program:5', '灯台の地図 #4', 'こたつ BS', '2026-11-22T12:00:00Z'),
            rules,
            reason: 'OTHER_BROADCAST',
            lastRecordedAt: null,
            recorded: null,
            instead: null,
          },
        ],
      },
    }));
    await renderWithRouter(<SkippedDuplicateList />, environment);

    expect(await screen.findByText('灯台の地図 に一致。2026/11/18 に録画済み')).toBeTruthy();
    expect(
      screen.getByText('灯台の地図 に一致。同じ回を こたつ BS で 2026/11/15 に録画済み'),
    ).toBeTruthy();
    expect(
      screen.getByText(
        /^灯台の地図 に一致。同じ回を こたつテスト1 の 11\/20\(金\) 21:00 の放送で録ります$/,
      ),
    ).toBeTruthy();
    expect(screen.getByText('灯台の地図 に一致。同じ回を別の放送で録ります')).toBeTruthy();
  });
});
