import { MantineProvider } from '@mantine/core';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { GoogleDriveSettings, type GoogleIntegrationItem } from './GoogleDriveSettings';

afterEach(cleanup);

const redirectUri = 'http://localhost:9090/internal/auth/google/callback';
const notConfigured: GoogleIntegrationItem = {
  clientId: null,
  configured: false,
  redirectUri,
  drive: null,
  folders: [],
};
const configured: GoogleIntegrationItem = {
  ...notConfigured,
  clientId: 'client',
  configured: true,
};
const connected: GoogleIntegrationItem = {
  ...configured,
  drive: { accountName: 'admin@example.com', connectedAt: '2026-11-17T03:00:00Z', revokedAt: null },
};

function renderSettings(
  integration: GoogleIntegrationItem,
  props: Partial<Parameters<typeof GoogleDriveSettings>[0]> = {},
) {
  const handlers = {
    onDismiss: vi.fn(),
    onSaveApp: vi.fn(),
    onConnect: vi.fn(),
    onDisconnect: vi.fn(),
    onAddFolder: vi.fn(),
    onRenameFolder: vi.fn(),
    onRemoveFolder: vi.fn(),
  };
  render(
    <MantineProvider env="test">
      <GoogleDriveSettings
        integration={integration}
        busy={false}
        connected={false}
        callbackError={null}
        {...handlers}
        {...props}
      />
    </MantineProvider>,
  );

  return handlers;
}

describe('GoogleDriveSettings (#1902)', () => {
  test('Google に登録するリダイレクト URI を出し、クライアントを登録するまで接続させない', () => {
    const handlers = renderSettings(notConfigured);
    expect(screen.getByText(redirectUri)).toBeDefined();
    expect(
      screen.getByText('接続するには、先にクライアントIDとシークレットを登録してください'),
    ).toBeDefined();
    const connect = screen.getByRole<HTMLButtonElement>('button', { name: '接続する' });
    expect(connect.disabled).toBe(true);
    expect(handlers.onConnect).not.toHaveBeenCalled();
  });

  test('シークレットを入れなければ、クライアント ID だけを送る', () => {
    const handlers = renderSettings(configured);
    fireEvent.change(screen.getByLabelText('クライアントID'), { target: { value: ' renamed ' } });
    fireEvent.click(screen.getByRole('button', { name: '登録' }));
    expect(handlers.onSaveApp).toHaveBeenCalledWith({ clientId: 'renamed' }, expect.any(Function));
  });

  test('接続していれば、アカウントを出し、外せる', () => {
    const handlers = renderSettings(connected);
    expect(screen.getByText(/admin@example\.com と接続しています/)).toBeDefined();
    fireEvent.click(screen.getByRole('button', { name: '接続を外す' }));
    expect(handlers.onDisconnect).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole('button', { name: '接続し直す' }));
    expect(handlers.onConnect).toHaveBeenCalledOnce();
  });

  test('接続したら、見せるフォルダを出し、外せる', () => {
    const handlers = renderSettings({
      ...connected,
      folders: [{ id: 'f1', name: '録画のバックアップ', libraryName: 'backup' }],
    });
    expect(screen.getByText('backup')).toBeDefined();
    expect(screen.getByText('フォルダ: 録画のバックアップ')).toBeDefined();
    fireEvent.click(screen.getByRole('button', { name: '外す' }));
    expect(handlers.onRemoveFolder).toHaveBeenCalledWith('f1');
    expect(screen.getByRole('button', { name: 'フォルダを追加' })).toBeDefined();
  });

  test('接続していないか切れていれば、見せるフォルダを選ばせない', () => {
    renderSettings(configured);
    expect(screen.queryByRole('button', { name: 'フォルダを追加' })).toBeNull();
    cleanup();
    renderSettings({
      ...connected,
      drive: { ...connected.drive!, revokedAt: '2026-11-18T03:00:00Z' },
    });
    expect(screen.queryByRole('button', { name: 'フォルダを追加' })).toBeNull();
  });

  test('Google で接続が切れたら、接続し直すよう伝える', () => {
    renderSettings({
      ...connected,
      drive: { ...connected.drive!, revokedAt: '2026-11-18T03:00:00Z' },
    });
    expect(screen.getByRole('alert').textContent).toMatch(/Googleで接続が切れました/);
  });

  test('戻ってきたときの結果を出す', () => {
    renderSettings(configured, { callbackError: 'google_denied' });
    expect(screen.getByRole('alert').textContent).toMatch(/Google Driveの読み取りを許さなかった/);
    cleanup();
    renderSettings(connected, { connected: true });
    expect(screen.getByRole('status').textContent).toMatch(/Google Driveを接続しました/);
  });
});
