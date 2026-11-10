import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { authApi } from '../auth/api';
import { unsubscribeDevicePush } from '../auth/logout';
import { removeOfflineRecording, savedOfflineRecordings } from '../offline/store';
import { DeleteAccount } from './DeleteAccount';
import { mutate } from './mutate';
import type { ReauthRun } from './useReauthGate';

vi.mock('react-relay', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-relay')>()),
  useRelayEnvironment: () => ({}),
}));
vi.mock('./mutate', () => ({ mutate: vi.fn() }));
vi.mock('../auth/logout', () => ({ unsubscribeDevicePush: vi.fn() }));
vi.mock('../offline/store', () => ({
  refreshOfflineRecordings: vi.fn(() => Promise.resolve()),
  removeOfflineRecording: vi.fn(() => Promise.resolve()),
  savedOfflineRecordings: vi.fn(() => []),
}));

const username = 'mikan';
const action = 'アカウントを削除';
const run: ReauthRun = (operation, onError) => {
  operation().catch(onError);
};

function openConfirmation(fail: (error: unknown) => void) {
  render(
    <MantineProvider>
      <DeleteAccount username={username} run={run} fail={fail} />
    </MantineProvider>,
  );
  fireEvent.click(screen.getByRole('button', { name: action }));
}

function confirmButton(): HTMLButtonElement {
  return within(screen.getByRole('dialog')).getByRole<HTMLButtonElement>('button', {
    name: action,
  });
}

function typeUsername(value: string) {
  fireEvent.change(screen.getByRole('textbox'), { target: { value } });
}

beforeEach(() => {
  vi.spyOn(authApi, 'logout').mockResolvedValue();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe('DeleteAccount (#1382)', () => {
  test('ユーザー名が一致するまで、消せない', () => {
    openConfirmation(vi.fn());

    expect(confirmButton().disabled).toBe(true);
    typeUsername('mika');
    expect(confirmButton().disabled).toBe(true);
    typeUsername(username);
    expect(confirmButton().disabled).toBe(false);
  });

  test('消した後に、この端末の購読と保存した録画を片付けて、ログアウトする', async () => {
    vi.mocked(mutate).mockResolvedValue({ deleteAccount: true } as never);
    vi.mocked(savedOfflineRecordings).mockReturnValue([{ id: 'Recording:1' }] as never);
    openConfirmation(vi.fn());
    typeUsername(username);

    fireEvent.click(confirmButton());

    await waitFor(() => expect(authApi.logout).toHaveBeenCalledOnce());
    expect(unsubscribeDevicePush).toHaveBeenCalledOnce();
    expect(removeOfflineRecording).toHaveBeenCalledWith('Recording:1');
  });

  test('消せなければ、端末のものは片付けず、エラーを伝える', async () => {
    const failure = new Error('BAD_USER_INPUT');
    vi.mocked(mutate).mockRejectedValue(failure);
    const fail = vi.fn();
    openConfirmation(fail);
    typeUsername(username);

    fireEvent.click(confirmButton());

    await waitFor(() => expect(fail).toHaveBeenCalledWith(failure));
    expect(unsubscribeDevicePush).not.toHaveBeenCalled();
    expect(authApi.logout).not.toHaveBeenCalled();
  });
});
