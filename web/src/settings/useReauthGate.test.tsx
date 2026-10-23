import { MantineProvider } from '@mantine/core';
import { act, cleanup, render, renderHook, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { AuthError } from '../auth/api';

import { useReauthGate } from './useReauthGate';

const methods = {
  totp: false,
  recoveryCode: false,
  passkey: false,
  oidc: { enabled: false, displayName: '', linked: false },
};

afterEach(() => {
  cleanup();
  window.history.replaceState(null, '', '/');
});

function Gate() {
  const gate = useReauthGate(methods, '/admin/system');

  return <MantineProvider>{gate.modal}</MantineProvider>;
}

describe('useReauthGate', () => {
  test('再認証を求められたら、onWaiting を呼んで待ち始め、操作の失敗としては扱わない (#915)', async () => {
    const { result } = renderHook(() => useReauthGate(methods, '/admin/system'));
    const onError = vi.fn();
    const onCancel = vi.fn();
    const onWaiting = vi.fn();

    await act(async () => {
      result.current.run(
        () => Promise.reject(new AuthError(403, 'REAUTHENTICATION_REQUIRED', undefined, undefined)),
        onError,
        onCancel,
        onWaiting,
      );
      await Promise.resolve();
    });

    expect(onWaiting).toHaveBeenCalledOnce();
    expect(onError).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();
    expect(result.current.busy).toBe(false);
  });

  test.each([
    { name: '成功した操作', operation: () => Promise.resolve(), wantError: false },
    {
      name: '再認証のほかの理由で失敗した操作',
      operation: () => Promise.reject(new AuthError(403, 'FORBIDDEN', undefined, undefined)),
      wantError: true,
    },
  ])('$name では、onWaiting を呼ばない', async ({ operation, wantError }) => {
    const { result } = renderHook(() => useReauthGate(methods, '/admin/system'));
    const onError = vi.fn();
    const onWaiting = vi.fn();

    await act(async () => {
      result.current.run(operation, onError, undefined, onWaiting);
      await Promise.resolve();
    });

    expect(onWaiting).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledTimes(wantError ? 1 : 0);
  });

  // ポップアップを止められ、この画面のまま IdP で再認証したときは、待っていた操作が消えるので、もう一度操作するよう知らせる (#916)。
  test.each([
    { name: '再認証を終えて戻った', search: '?reauthenticated=oidc', want: 'status' },
    { name: '再認証に失敗して戻った', search: '?error=oidc_denied', want: 'alert' },
  ])('$nameときは、知らせを出す', ({ search, want }) => {
    window.history.replaceState(null, '', `/admin/system${search}`);
    render(<Gate />);

    expect(screen.getByRole(want)).toBeTruthy();
  });

  test('IdP から戻ったのでなければ、知らせを出さない', () => {
    window.history.replaceState(null, '', '/admin/system');
    render(<Gate />);

    expect(screen.queryByRole('status')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
