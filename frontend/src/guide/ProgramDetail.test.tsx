import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { generatePrograms } from '../demo/epg';
import { createDemoEnvironment, renderWithRouter } from '../test/route';
import { ProgramDetail } from './ProgramDetail';

const now = Date.UTC(2026, 10, 11, 3, 0);

// 番組の詳細は、放送が終わったか、予約できるかを実際の時計で決めるので、時計を now に止める。
// 止めないと、選んだ番組が実際の日付で過去になったときに、予約のボタンが出ずに落ちる。
// 待ち合わせのタイマーは本物のまま使うので、止めるのは Date だけにする。
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] });
  vi.setSystemTime(now);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

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
