import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import '../i18n';
import { setTimeZone } from '../futon/timeZone';
import { RecordedJikkyoList } from './RecordedJikkyoList';

afterEach(() => {
  cleanup();
});

const comments = [
  { time: 10, content: 'はじまった', mail: '' },
  { time: 70, content: 'つぎ', mail: '' },
  { time: 130, content: 'まだ流れていない', mail: '' },
];

describe('RecordedJikkyoList', () => {
  test.each([
    {
      name: '録り始めが分かれば、放送時刻で出す。途切れた区間の分は時刻を進める',
      recordedFrom: '2026-11-28T12:00:00Z',
      gaps: [{ from: '2026-11-28T12:00:30Z', to: '2026-11-28T12:01:30Z' }],
      want: ['21:00:10', '21:02:10'],
    },
    {
      name: '録り始めが分からなければ、再生の経過時間で出す',
      recordedFrom: null,
      gaps: [],
      want: ['0:10', '1:10'],
    },
  ])('$name', ({ recordedFrom, gaps, want }) => {
    setTimeZone('Asia/Tokyo');
    render(
      <MantineProvider env="test">
        <RecordedJikkyoList
          comments={comments}
          count={2}
          offset={0}
          recordedFrom={recordedFrom}
          gaps={gaps}
          frozen={false}
          onMuteUser={() => undefined}
        />
      </MantineProvider>,
    );

    for (const label of want) {
      expect(screen.getByText(label)).toBeTruthy();
    }
    expect(screen.queryByText('まだ流れていない')).toBeNull();
  });
});
