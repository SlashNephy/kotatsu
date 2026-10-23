import { MantineProvider } from '@mantine/core';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { authApi } from './api';
import { reportOIDCReauthentication } from './oidcReauth';
import { Reauthenticate } from './Reauthenticate';

const methods = {
  totp: false,
  recoveryCode: false,
  passkey: false,
  oidc: { enabled: true, displayName: 'Dex', linked: true },
};

// popupStub は、window.open が返す、IdP へ送り出すウィンドウの代わり。
function popupStub() {
  return { location: { href: '' }, close: vi.fn() };
}

const before = '2026-10-23T03:00:00Z';

beforeEach(() => {
  vi.spyOn(authApi, 'beginOIDCReauthentication').mockResolvedValue({
    authorizationUrl: 'https://idp.example/authorize',
  });
  vi.spyOn(authApi, 'reauthenticationStatus').mockResolvedValue({ authenticatedAt: before });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderReauthenticate(onDone: () => void) {
  render(
    <MantineProvider>
      <Reauthenticate methods={methods} returnTo="/admin/system" onDone={onDone} />
    </MantineProvider>,
  );
}

describe('Reauthenticate の IdP での再認証 (#916)', () => {
  test('別のウィンドウで IdP へ送り出し、終えたと知らされたら onDone を呼ぶ', async () => {
    const popup = popupStub();
    vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    const onDone = vi.fn();
    renderReauthenticate(onDone);

    fireEvent.click(screen.getByRole('button', { name: 'Dexで確認' }));
    await waitFor(() => {
      expect(popup.location.href).toBe('https://idp.example/authorize');
    });
    const [returnTo] = vi.mocked(authApi.beginOIDCReauthentication).mock.calls[0]!;
    const id = new URL(returnTo, 'http://localhost').searchParams.get('id')!;
    expect(returnTo).toBe(`/reauth/done?id=${id}`);
    expect(onDone).not.toHaveBeenCalled();

    // ほかの再認証の知らせでは、続けない。
    act(() => reportOIDCReauthentication({ id: 'another', error: undefined }));
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(onDone).not.toHaveBeenCalled();

    act(() => reportOIDCReauthentication({ id, error: undefined }));
    await waitFor(() => {
      expect(onDone).toHaveBeenCalledOnce();
    });
  });

  test('IdP で失敗したと知らされたら、理由を出し、onDone を呼ばない', async () => {
    const popup = popupStub();
    vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    const onDone = vi.fn();
    renderReauthenticate(onDone);

    fireEvent.click(screen.getByRole('button', { name: 'Dexで確認' }));
    await waitFor(() => {
      expect(popup.location.href).toBe('https://idp.example/authorize');
    });
    const [returnTo] = vi.mocked(authApi.beginOIDCReauthentication).mock.calls[0]!;
    const id = new URL(returnTo, 'http://localhost').searchParams.get('id')!;

    act(() => reportOIDCReauthentication({ id, error: 'oidc_denied' }));
    expect(await screen.findByRole('alert')).toBeTruthy();
    expect(onDone).not.toHaveBeenCalled();
  });

  test('知らせが届かなくても、元の画面に戻ったときに認証の日時が変わっていれば onDone を呼ぶ', async () => {
    const popup = popupStub();
    vi.spyOn(window, 'open').mockReturnValue(popup as unknown as Window);
    const onDone = vi.fn();
    renderReauthenticate(onDone);

    fireEvent.click(screen.getByRole('button', { name: 'Dexで確認' }));
    await waitFor(() => {
      expect(popup.location.href).toBe('https://idp.example/authorize');
    });
    // 認証の日時が変わっていなければ、続けない。
    act(() => {
      window.dispatchEvent(new Event('focus'));
    });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(onDone).not.toHaveBeenCalled();

    vi.mocked(authApi.reauthenticationStatus).mockResolvedValue({
      authenticatedAt: '2026-10-23T03:05:00Z',
    });
    act(() => {
      window.dispatchEvent(new Event('focus'));
    });
    await waitFor(() => {
      expect(onDone).toHaveBeenCalledOnce();
    });
  });

  test('ウィンドウを開けなければ、この画面のまま IdP へ送り出し、この画面に戻させる', async () => {
    vi.spyOn(window, 'open').mockReturnValue(null);
    const assign = vi.fn();
    vi.spyOn(window, 'location', 'get').mockReturnValue({ assign } as unknown as Location);
    renderReauthenticate(vi.fn());

    fireEvent.click(screen.getByRole('button', { name: 'Dexで確認' }));

    await waitFor(() => {
      expect(assign).toHaveBeenCalledWith('https://idp.example/authorize');
    });
    expect(authApi.beginOIDCReauthentication).toHaveBeenCalledWith('/admin/system');
  });
});
