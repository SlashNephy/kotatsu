import { act, cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import { DataChangesProvider, useLiveScrambledDispatch } from '../changes/DataChanges';
import { useLiveScrambled } from './useLiveScrambled';

let send: ReturnType<typeof useLiveScrambledDispatch> = () => {};

function Sender() {
  send = useLiveScrambledDispatch();

  return null;
}

function Player({ serviceId, onScrambled }: { serviceId: string; onScrambled: () => void }) {
  useLiveScrambled(serviceId, onScrambled);

  return null;
}

afterEach(() => {
  cleanup();
});

describe('useLiveScrambled', () => {
  test('自分のサービスの知らせが届いたときだけ、届くたびに呼ぶ', () => {
    const onScrambled = vi.fn();
    render(
      <DataChangesProvider>
        <Sender />
        <Player serviceId="Service:1" onScrambled={onScrambled} />
      </DataChangesProvider>,
    );

    act(() => send({ service: 'Service:2', decidedAt: '2026-11-12T12:00:00Z' }));
    expect(onScrambled).not.toHaveBeenCalled();

    act(() => send({ service: 'Service:1', decidedAt: '2026-11-12T12:00:00Z' }));
    expect(onScrambled).toHaveBeenCalledTimes(1);

    act(() => send({ service: 'Service:1', decidedAt: '2026-11-12T12:01:00Z' }));
    expect(onScrambled).toHaveBeenCalledTimes(2);
  });

  test('画面を開く前に届いた知らせでは呼ばない', () => {
    const onScrambled = vi.fn();
    const { rerender } = render(
      <DataChangesProvider>
        <Sender />
      </DataChangesProvider>,
    );
    act(() => send({ service: 'Service:1', decidedAt: '2026-11-12T12:00:00Z' }));

    rerender(
      <DataChangesProvider>
        <Sender />
        <Player serviceId="Service:1" onScrambled={onScrambled} />
      </DataChangesProvider>,
    );
    expect(onScrambled).not.toHaveBeenCalled();
  });
});
