import { act, cleanup, fireEvent, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { createRootValue } from '../../demo/data';
import { createEventHub } from '../../demo/events';
import { createExecutor } from '../../demo/executor';
import { createDemoEnvironment, createTestEnvironment, renderRoute } from '../../test/route';
import { Route } from './$recordingId';

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// completedRecording は、時刻 now のデモで、録画を終えた録画の ID を返す。
async function completedRecording(now: number): Promise<string> {
  const result = await createExecutor(createRootValue(createEventHub(), now)).execute(
    `query { recordings(first: 100) { edges { node { id state } } } }`,
    {},
  );
  const edges = (
    result.data as { recordings: { edges: { node: { id: string; state: string } }[] } }
  ).recordings.edges;

  return edges.find((edge) => edge.node.state === 'COMPLETED')!.node.id;
}

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

  test('CM の解析を頼むと、待っている、解析している、終えたを、読み直して出す (#1571)', async () => {
    // デモは、頼んでから 3 秒待ち、15 秒で解析し終える。デモの映像は CM を含まないので、区間は見つからない。
    const now = Date.now();
    const recordingId = await completedRecording(now);
    await renderRoute(
      Route,
      '/recordings/$recordingId',
      `/recordings/${recordingId}`,
      createDemoEnvironment({}, { now }),
    );
    expect(await screen.findByText('まだ解析していません')).toBeTruthy();
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });

    fireEvent.click(screen.getByRole('button', { name: 'CMを解析する' }));
    await act(() => vi.advanceTimersByTimeAsync(0));
    expect(screen.getByText('解析を待っています')).toBeTruthy();

    // 終わったことは、変更の知らせが届かなくても、読み直して出す。
    await act(() => vi.advanceTimersByTimeAsync(5_000));
    expect(screen.getByText('解析しています')).toBeTruthy();
    expect(screen.getByRole('progressbar', { name: '解析しています' })).toBeTruthy();

    await act(() => vi.advanceTimersByTimeAsync(15_000));
    expect(screen.getByText('CMは見つかりませんでした')).toBeTruthy();
    expect(screen.queryByText('解析しています')).toBeNull();
  });
});
