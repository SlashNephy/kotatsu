import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createDemoEnvironment, createFailingEnvironment, renderRoute } from '../../test/route';
import { Route } from './$serviceId_.play';

// TsPlayer は jsdom で動かないので、映像と操作バーだけを描く部品に置き換える。
vi.mock('../../yunomi/TsPlayer', () => ({
  TsPlayer: ({ controls }: { controls: ReactNode }) => (
    <>
      <video />
      {controls}
    </>
  ),
}));

// jsdom には MediaSource がないので、ブラウザの中で変換できる端末として振る舞う。
vi.mock('../../yunomi/capabilities', () => ({
  detectCapabilities: () => ({ convert: true, hls: true, hevc: true, hevcUhd: false }),
}));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

const startAt = '2026-11-11T00:00:00+09:00';

describe('timeshift PlayPage', () => {
  test('スリープタイマーの時間が来たら、プレイヤーを閉じてライブ視聴の画面へ戻る (#1426)', async () => {
    const environment = createDemoEnvironment({
      timeshiftProgram: () => ({
        name: '番組',
        startAt,
        endAt: '2026-11-11T01:00:00+09:00',
        playableFrom: startAt,
        gaps: [],
        streamUrl: '/stream.m2ts',
        hlsUrl: '/stream.m3u8',
        service: { id: 'Service:1', name: 'サービス', logoUrl: null, videoFormat: 'HD' },
      }),
    });
    const { router } = await renderRoute(
      Route,
      '/timeshift/$serviceId_/play',
      `/timeshift/Service:1/play?program=${encodeURIComponent(startAt)}`,
      environment,
    );
    await screen.findByRole('button', { name: '設定' });
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });

    fireEvent.click(screen.getByRole('button', { name: '設定' }));
    fireEvent.click(await screen.findByRole('combobox', { name: 'スリープタイマー' }));
    fireEvent.click(await screen.findByRole('option', { name: '15 分後に止める' }));
    await act(() => vi.advanceTimersByTimeAsync(15 * 60_000 + 1_000));

    expect(router.state.location.pathname).toBe('/live/Service%3A1');
  });

  test('エラーの後に同じサービスの別の番組へ移ると、エラーの表示を消して読み直す (#1453)', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    // 1 つ目の番組は読み込みに失敗する。2 つ目の番組は、応答を返さず読み込み中のままにする。
    const nextStartAt = '2026-11-11T01:00:00+09:00';
    const { environment, requested } = createFailingEnvironment(startAt);
    const { router } = await renderRoute(
      Route,
      '/timeshift/$serviceId_/play',
      `/timeshift/Service:1/play?program=${encodeURIComponent(startAt)}`,
      environment,
    );
    const loadError = '読み込めませんでした。時間をおいてやり直してください';
    expect(await screen.findByText(loadError)).toBeTruthy();

    await act(() =>
      router.navigate({
        to: '/timeshift/$serviceId/play',
        params: { serviceId: 'Service:1' },
        search: { program: nextStartAt },
      }),
    );

    expect(screen.queryByText(loadError)).toBeNull();
    expect(requested.some((variables) => variables.includes(nextStartAt))).toBe(true);
  });
});
