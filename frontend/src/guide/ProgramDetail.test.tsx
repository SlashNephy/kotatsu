import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { generatePrograms } from '../demo/epg';
import { createDemoEnvironment, renderWithRouter } from '../test/route';
import { ProgramDetail } from './ProgramDetail';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const now = Date.UTC(2026, 10, 11, 3, 0);

// Harness は、番組の詳細を開き閉じするボタンを置く。
function Harness({ programId }: { programId: string }) {
  const [opened, setOpened] = useState(true);

  return (
    <>
      <button type="button" onClick={() => setOpened(true)}>
        開く
      </button>
      <ProgramDetail programId={opened ? programId : undefined} onClose={() => setOpened(false)} />
    </>
  );
}

describe('ProgramDetail', () => {
  test('予約した番組を開き直すと、読み直しを待たずに予約済みを出す (#1421)', async () => {
    // 2 日後に始まる番組を、録画予約していない状態から予約する。
    const program = generatePrograms(now).find(
      (item) => item.start > now + 2 * 24 * 60 * 60_000 && item.isFree,
    )!;
    let opens = 0;
    const environment = createDemoEnvironment(
      {},
      {
        now,
        // 開き直したときの読み直しには、応答を返さない。
        hold: (request) => request.name === 'ProgramDetailQuery' && ++opens > 1,
      },
    );
    await renderWithRouter(<Harness programId={program.id} />, environment);
    fireEvent.click(await screen.findByRole('button', { name: '録画予約' }));
    // 予約すると、詳細を閉じる。
    await vi.waitFor(() => expect(screen.queryByText(program.name)).toBeNull());

    await act(async () => fireEvent.click(screen.getByRole('button', { name: '開く' })));

    expect(screen.queryByText('読み込んでいます')).toBeNull();
    expect(screen.getByRole('button', { name: '録画予約済み' })).toBeTruthy();
  });
});
