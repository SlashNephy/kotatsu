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

describe('useReauthGate', () => {
  test('再認証を求められたら、onWaiting を呼んで待ち始め、操作の失敗としては扱わない (#915)', async () => {
    const { result } = renderHook(() => useReauthGate(methods, '/admin/storage'));
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
    const { result } = renderHook(() => useReauthGate(methods, '/admin/storage'));
    const onError = vi.fn();
    const onWaiting = vi.fn();

    await act(async () => {
      result.current.run(operation, onError, undefined, onWaiting);
      await Promise.resolve();
    });

    expect(onWaiting).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalledTimes(wantError ? 1 : 0);
  });
});

// Gate は、再認証の画面を出す画面の代わり。
function Gate() {
  const gate = useReauthGate(methods, '/admin/storage');

  return <MantineProvider>{gate.modal}</MantineProvider>;
}

describe('IdP での再認証から戻ったとき', () => {
  test('失敗して戻ったときは、理由を出す', () => {
    window.history.replaceState(null, '', '/admin/storage?error=oidc_denied');

    render(<Gate />);

    expect(screen.getByRole('alert')).toBeTruthy();
  });

  test('成功して戻ったときは、何も出さない (開き直した画面は SudoGate が確かめる)', () => {
    window.history.replaceState(null, '', '/admin/storage');

    render(<Gate />);

    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();
  });
});
