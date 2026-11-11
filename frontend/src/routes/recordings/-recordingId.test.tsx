import { act, cleanup, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './$recordingId';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('RecordingPage', () => {
  test('エラーの後に別の録画へ移ると、エラーの表示を消して読み直す (#1422)', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    // 1 つ目の録画は見つからない。2 つ目の録画は、応答を返さず読み込み中のままにする。
    const requested: unknown[] = [];
    const environment = createTestEnvironment((_, variables) => {
      requested.push(variables.id);

      return variables.id === 'Recording:1'
        ? { data: { viewer: { role: 'ADMIN' }, tags: [], node: null } }
        : undefined;
    });
    const { router } = await renderRoute(
      Route,
      '/recordings/$recordingId',
      '/recordings/Recording:1',
      environment,
    );
    expect(await screen.findByText('録画が見つかりませんでした')).toBeTruthy();

    await act(() =>
      router.navigate({ to: '/recordings/$recordingId', params: { recordingId: 'Recording:2' } }),
    );

    expect(screen.queryByText('録画が見つかりませんでした')).toBeNull();
    expect(requested).toContain('Recording:2');
  });
});
