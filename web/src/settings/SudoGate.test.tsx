import { MantineProvider } from '@mantine/core';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { authApi } from '../auth/api';
import { SudoGate } from './SudoGate';

const methods = {
  totp: true,
  recoveryCode: false,
  passkey: false,
  oidc: { enabled: false, displayName: '', linked: false },
};

const sudoDescription =
  'ここには重要な操作があるので、出す前に、本人であることをもう一度確かめます。';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderGate() {
  render(
    <MantineProvider>
      <SudoGate methods={methods} returnTo="/settings/security">
        <p>重要な操作</p>
      </SudoGate>
    </MantineProvider>,
  );
}

describe('SudoGate (#981)', () => {
  test('直近に認証していれば、そのまま中身を出す', async () => {
    vi.spyOn(authApi, 'reauthenticationStatus').mockResolvedValue({
      authenticatedAt: '2026-10-23T03:00:00Z',
      recent: true,
    });

    renderGate();

    expect(await screen.findByText('重要な操作')).toBeTruthy();
    expect(screen.queryByText(sudoDescription)).toBeNull();
  });

  test('再認証が要るなら、中身の代わりに本人確認を出す', async () => {
    vi.spyOn(authApi, 'reauthenticationStatus').mockResolvedValue({
      authenticatedAt: '2026-10-23T03:00:00Z',
      recent: false,
    });

    renderGate();

    expect(await screen.findByText(sudoDescription)).toBeTruthy();
    expect(screen.queryByText('重要な操作')).toBeNull();
  });

  test('確かめられなければ、中身を出す (必要なら、操作のときに再認証を求められる)', async () => {
    vi.spyOn(authApi, 'reauthenticationStatus').mockRejectedValue(new Error('offline'));

    renderGate();

    expect(await screen.findByText('重要な操作')).toBeTruthy();
  });
});
