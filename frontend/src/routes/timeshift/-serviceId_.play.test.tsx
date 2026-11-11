import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createDemoEnvironment, renderRoute } from '../../test/route';
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
});
