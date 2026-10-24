import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import {
  DataChangesProvider,
  settleMs,
  useDataChangeDispatch,
  useDataChanges,
} from './DataChanges';
import type { ChangeChannel } from './dataChanges';

let dispatch: ReturnType<typeof useDataChangeDispatch> = () => {};

function Probe({ channels = ['RULES'] }: { channels?: readonly ChangeChannel[] }) {
  dispatch = useDataChangeDispatch();
  const key = useDataChanges(channels);

  return <span data-testid="key">{key}</span>;
}

function send(channel: ChangeChannel, version: number) {
  act(() => dispatch({ channel, version }));
}

function wait(ms: number) {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('useDataChanges', () => {
  test('続けて届いた変更は、届かなくなってから settleMs 待って、まとめて 1 回だけ反映する', () => {
    render(
      <DataChangesProvider>
        <Probe />
      </DataChangesProvider>,
    );
    const key = () => screen.getByTestId('key').textContent;
    send('RULES', 1);
    expect(key()).toBe('0');

    send('RULES', 2);
    wait(settleMs - 500);
    send('RULES', 3);
    wait(settleMs - 500);
    // 最後の変更から settleMs たっていないので、まだ変えない。
    expect(key()).toBe('0');

    wait(500);
    expect(key()).toBe('2');
  });

  test('見ていない種類が変わっても、値を変えない', () => {
    render(
      <DataChangesProvider>
        <Probe />
      </DataChangesProvider>,
    );
    send('EPG', 1);
    send('EPG', 2);
    wait(settleMs);

    expect(screen.getByTestId('key').textContent).toBe('0');
  });

  test('ログイン中のユーザーの設定の変更も、データの変更と同じく数える', () => {
    render(
      <DataChangesProvider>
        <Probe channels={['VIEWER_SETTINGS']} />
      </DataChangesProvider>,
    );
    // 購読を始めたときに届く今の版数では、読み直さない。
    send('VIEWER_SETTINGS', 7);
    wait(settleMs);
    expect(screen.getByTestId('key').textContent).toBe('0');

    send('VIEWER_SETTINGS', 8);
    send('RULES', 1);
    send('RULES', 2);
    wait(settleMs);
    expect(screen.getByTestId('key').textContent).toBe('1');
  });
});
