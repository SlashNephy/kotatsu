import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, test } from 'vitest';

import { createTestEnvironment, renderWithRouter } from '../test/route';
import { NextEpisodes, type NextEpisode } from './NextEpisodes';

afterEach(() => {
  cleanup();
});

// episode は、続きの放送の番組。
function episode(id: string, name: string, serviceId: string, startAt: string): NextEpisode {
  return {
    id,
    name,
    startAt,
    ruleKeyword: '霧笛の港',
    service: { id: serviceId, name: serviceId.replace('Service:', ''), logoUrl: null },
  };
}

const episodes = [
  episode('Program:1', '霧笛の港 #4', 'Service:こたつ BS', '2026-11-20T12:00:00Z'),
  episode('Program:2', '霧笛の港 #4', 'Service:こたつテスト1', '2026-11-21T12:00:00Z'),
];

// Refetching は、「読み直す」を押すと、予約した放送を外したサーバーの応答 (空の一覧) に変わる録画の詳細。
function Refetching() {
  const [current, setCurrent] = useState<readonly NextEpisode[]>(episodes);

  return (
    <>
      <button type="button" onClick={() => setCurrent([])}>
        読み直す
      </button>
      <NextEpisodes episodes={current} serviceId="Service:こたつテスト1" canEdit />
    </>
  );
}

describe('続きの放送 (#1564)', () => {
  test('続きの放送がなければ、何も出さない', async () => {
    await renderWithRouter(
      <NextEpisodes episodes={[]} serviceId="Service:こたつテスト1" canEdit />,
      createTestEnvironment(() => undefined),
    );

    expect(screen.queryByRole('region', { name: '続きの放送' })).toBeNull();
  });

  test('放送の予約を押すと、その番組を予約し、読み直して外れても予約済みとして出し続ける', async () => {
    const requested: unknown[] = [];
    const environment = createTestEnvironment((request, variables) => {
      requested.push({ name: request.name, variables });

      return { data: { createProgramReservation: { id: 'Reservation:1' } } };
    });
    await renderWithRouter(<Refetching />, environment);

    fireEvent.click(screen.getAllByRole('button', { name: '霧笛の港 #4を予約' })[1]!);

    expect(await screen.findByText('予約済み')).toBeTruthy();
    expect(requested).toEqual([
      { name: 'NextEpisodesReserveMutation', variables: { program: 'Program:2' } },
    ]);
    // サーバーは予約した放送を外して返すので、続きの放送がなくなっても、予約した放送は出し続ける。
    fireEvent.click(screen.getByRole('button', { name: '読み直す' }));
    expect(screen.getByRole('region', { name: '続きの放送' })).toBeTruthy();
    expect(screen.getByText('予約済み')).toBeTruthy();
    expect(screen.queryByRole('button', { name: '霧笛の港 #4を予約' })).toBeNull();
  });

  test('予約できなければ、理由を出し、予約のボタンを残す', async () => {
    // 番組がもう予約されているなどで、サーバーが断る。
    const environment = createTestEnvironment(() => ({
      data: { createProgramReservation: null },
      errors: [{ message: 'conflict' }],
    }));
    await renderWithRouter(
      <NextEpisodes episodes={episodes} serviceId="Service:こたつテスト1" canEdit />,
      environment,
    );

    fireEvent.click(screen.getAllByRole('button', { name: '霧笛の港 #4を予約' })[0]!);

    expect(await screen.findByRole('alert')).toHaveProperty('textContent', '予約できませんでした');
    expect(screen.queryByText('予約済み')).toBeNull();
    expect(screen.getAllByRole('button', { name: '霧笛の港 #4を予約' })).toHaveLength(2);
  });

  test('ルールは、録画の局の放送の作品名と局を条件にして、ルールの作成画面で開く', async () => {
    const { router } = await renderWithRouter(
      <NextEpisodes episodes={episodes} serviceId="Service:こたつテスト1" canEdit />,
      createTestEnvironment(() => undefined),
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'このシリーズのルールを作る' }));
      await Promise.resolve();
    });

    expect(router.state.location.pathname).toBe('/rules/new');
    const conditions = (
      router.state.location.state as {
        ruleConditions: { keywords: { text: string }[]; services: string[] };
      }
    ).ruleConditions;
    expect(conditions.keywords.map((keyword) => keyword.text)).toEqual(['霧笛の港']);
    expect(conditions.services).toEqual(['Service:こたつテスト1']);
  });

  test('視聴者には、予約とルールのボタンを出さない', async () => {
    await renderWithRouter(
      <NextEpisodes episodes={episodes} serviceId="Service:こたつテスト1" canEdit={false} />,
      createTestEnvironment(() => undefined),
    );

    expect(screen.getByRole('region', { name: '続きの放送' })).toBeTruthy();
    expect(screen.queryByRole('button')).toBeNull();
  });
});
