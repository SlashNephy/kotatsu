import { act, cleanup, fireEvent, screen, within } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderWithRouter } from '../test/route';
import { NextEpisodesPanel, type SeriesNextEpisode } from './NextEpisodesPanel';

afterEach(() => {
  cleanup();
});

// suggestion は、見た録画 watched のシリーズの、次の放送。
function suggestion(
  watched: string,
  id: string,
  name: string,
  ruleKeyword: string,
  serviceId: string,
  startAt: string,
): SeriesNextEpisode {
  return {
    watched: { id: watched },
    program: {
      id,
      name,
      startAt,
      ruleKeyword,
      service: { id: serviceId, name: serviceId.replace('Service:', ''), logoUrl: null },
    },
  };
}

const harbor = suggestion(
  'Recording:1',
  'Program:42',
  '霧笛の港 #42',
  '霧笛の港',
  'Service:こたつテスト1',
  '2026-11-21T12:00:00Z',
);
const lighthouse = suggestion(
  'Recording:2',
  'Program:7',
  '灯台の地図 #7',
  '灯台の地図',
  'Service:こたつ BS',
  '2026-11-20T12:00:00Z',
);
// harborNext は、#42 を予約した後に、サーバーが同じシリーズの次の回として返す放送。
const harborNext = suggestion(
  'Recording:1',
  'Program:43',
  '霧笛の港 #43',
  '霧笛の港',
  'Service:こたつテスト1',
  '2026-11-22T12:00:00Z',
);

// Refetching は、「読み直す」を押すと、予約した #42 を外して #43 を返すサーバーの応答に変わるホーム。
function Refetching() {
  const [current, setCurrent] = useState<readonly SeriesNextEpisode[]>([harbor, lighthouse]);

  return (
    <>
      <button type="button" onClick={() => setCurrent([harborNext, lighthouse])}>
        読み直す
      </button>
      <NextEpisodesPanel suggestions={current} canEdit />
    </>
  );
}

function rowNames() {
  return within(screen.getByRole('region', { name: '続きの放送' }))
    .getAllByText(/#\d+/)
    .map((element) => element.textContent);
}

describe('ホームの続きの放送 (#1564)', () => {
  test('見たシリーズの続きの放送がなければ、何も出さない', async () => {
    await renderWithRouter(
      <NextEpisodesPanel suggestions={[]} canEdit />,
      createTestEnvironment(() => undefined),
    );

    expect(screen.queryByRole('region', { name: '続きの放送' })).toBeNull();
  });

  test('シリーズごとの次の放送を、放送の早い順に並べる', async () => {
    await renderWithRouter(
      <NextEpisodesPanel suggestions={[harbor, lighthouse]} canEdit />,
      createTestEnvironment(() => undefined),
    );

    expect(rowNames()).toEqual(['灯台の地図 #7', '霧笛の港 #42']);
  });

  test('予約すると、読み直して同じシリーズの次の回に変わっても、予約した放送を予約済みとして出し続ける', async () => {
    const requested: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      requested.push({ name: request.name, variables });

      return { data: { createProgramReservation: { id: 'Reservation:1' } } };
    });
    await renderWithRouter(<Refetching />, environment);

    fireEvent.click(screen.getByRole('button', { name: '霧笛の港 #42を予約' }));

    expect(await screen.findByText('予約済み')).toBeTruthy();
    expect(requested).toEqual([
      { name: 'NextEpisodesReserveMutation', variables: { program: 'Program:42' } },
    ]);
    fireEvent.click(screen.getByRole('button', { name: '読み直す' }));
    expect(rowNames()).toEqual(['灯台の地図 #7', '霧笛の港 #42']);
    expect(screen.getByText('予約済み')).toBeTruthy();
    expect(screen.getByRole('button', { name: '灯台の地図 #7を予約' })).toBeTruthy();
  });

  test('ルールは、その行の放送の作品名と局を条件にして、ルールの作成画面で開く', async () => {
    const { router } = await renderWithRouter(
      <NextEpisodesPanel suggestions={[harbor, lighthouse]} canEdit />,
      createTestEnvironment(() => undefined),
    );

    await act(async () => {
      fireEvent.click(
        screen.getByRole('button', { name: '灯台の地図 #7のシリーズのルールを作る' }),
      );
      await Promise.resolve();
    });

    expect(router.state.location.pathname).toBe('/rules/new');
    const conditions = (
      router.state.location.state as {
        ruleConditions: { keywords: { text: string }[]; services: string[] };
      }
    ).ruleConditions;
    expect(conditions.keywords.map((keyword) => keyword.text)).toEqual(['灯台の地図']);
    expect(conditions.services).toEqual(['Service:こたつ BS']);
  });

  test('視聴者には、予約とルールのボタンを出さない', async () => {
    await renderWithRouter(
      <NextEpisodesPanel suggestions={[harbor, lighthouse]} canEdit={false} />,
      createTestEnvironment(() => undefined),
    );

    expect(rowNames()).toEqual(['灯台の地図 #7', '霧笛の港 #42']);
    expect(screen.queryByRole('button')).toBeNull();
  });
});
