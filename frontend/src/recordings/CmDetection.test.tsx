import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test } from 'vitest';

import '../i18n';
import { CmDetectionStatus, type CmDetectionValue } from './CmDetection';

afterEach(cleanup);

function renderStatus(detection: CmDetectionValue, sections: number) {
  render(
    <MantineProvider env="test">
      <CmDetectionStatus detection={detection} sections={sections} />
    </MantineProvider>,
  );

  return screen.getByRole('status').textContent;
}

const detectedAt = '2026-11-12T10:30:00+09:00';

describe('CmDetectionStatus', () => {
  test.each<{ name: string; detection: CmDetectionValue; sections: number; want: string[] }>([
    {
      name: '見分け終えたら、CM とみなした区間の数を出す',
      detection: { state: 'DETECTED', detectedAt, progress: null },
      sections: 4,
      want: ['4か所をCMとみなしました', '11/12 10:30に解析しました'],
    },
    {
      name: '見分け終えて区間がなければ、見つからなかったと出す',
      detection: { state: 'DETECTED', detectedAt, progress: null },
      sections: 0,
      want: ['CMは見つかりませんでした'],
    },
    {
      name: '見分けている間は、進み具合と残り時間を出し、前に見分けた結果を使っていると添える',
      detection: {
        state: 'RUNNING',
        detectedAt,
        progress: { ratio: 0.426, remainingSeconds: 150 },
      },
      sections: 2,
      want: [
        '解析しています',
        '42% ・ 残り約3分',
        'いまは、11/12 10:30に解析した結果（2か所）を使っています',
      ],
    },
    {
      name: '見分け直しに失敗したら、前に見分けた結果を使っていると添える',
      detection: { state: 'FAILED', detectedAt, progress: null },
      sections: 3,
      want: ['解析できませんでした', 'いまは、11/12 10:30に解析した結果（3か所）を使っています'],
    },
  ])('$name', ({ detection, sections, want }) => {
    const text = renderStatus(detection, sections);

    for (const item of want) {
      expect(text).toContain(item);
    }
  });

  test('見分けていない録画では、前の結果を添えない', () => {
    const text = renderStatus({ state: 'NOT_DETECTED', detectedAt: null, progress: null }, 0);

    expect(text).toBe('まだ解析していません');
  });
});
