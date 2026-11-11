import { act, cleanup, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createFailingEnvironment, renderRoute } from '../../test/route';
import { Route } from './$recordingId_.play';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('recordings PlayPage', () => {
  test('エラーの後に別の録画へ移ると、エラーの表示を消して読み直す (#1453)', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    // 1 つ目は読み込みに失敗する。2 つ目は、応答を返さず読み込み中のままにする。
    const { environment, requested } = createFailingEnvironment('Recording:1');
    const { router } = await renderRoute(
      Route,
      '/recordings/$recordingId_/play',
      '/recordings/Recording:1/play',
      environment,
    );
    expect(
      await screen.findByText('読み込めませんでした。時間をおいてやり直してください'),
    ).toBeTruthy();

    await act(() =>
      router.navigate({
        to: '/recordings/$recordingId/play',
        params: { recordingId: 'Recording:2' },
      }),
    );

    expect(screen.queryByText('読み込めませんでした。時間をおいてやり直してください')).toBeNull();
    expect(requested.some((variables) => variables.includes('Recording:2'))).toBe(true);
  });
});
