import { MantineProvider } from '@mantine/core';
import { act, cleanup, fireEvent, render, renderHook, screen } from '@testing-library/react';
import { afterEach, describe, expect, test, vi } from 'vitest';

import '../i18n';
import { authApi, AuthError } from '../auth/api';

import { useReauthBeforeInput, useReauthGate, useReauthResume } from './useReauthGate';

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
});

const storageKey = 'kotatsu.reauth.resume';
const reauthRequired = () =>
  Promise.reject(new AuthError(403, 'REAUTHENTICATION_REQUIRED', undefined, undefined));

function stored(key: string | null, path = '/admin/system', params?: string) {
  window.sessionStorage.setItem(
    storageKey,
    JSON.stringify({
      key,
      path,
      savedAt: performance.timeOrigin - 1000,
      ...(params === undefined ? {} : { params }),
    }),
  );
}

// Page は、続け方を登録した操作のある画面の代わり。
function Page({ onResume }: { onResume: (params: string | undefined) => void }) {
  const gate = useReauthGate(methods, '/admin/system');
  useReauthResume('webhook.delete', onResume);

  return <MantineProvider>{gate.modal}</MantineProvider>;
}

describe('IdP での再認証から戻ったときの再開 (#916)', () => {
  afterEach(() => {
    window.sessionStorage.clear();
  });

  test('再認証を求められたら、操作の種類と対象を覚えておく', async () => {
    window.history.replaceState(null, '', '/admin/system');
    const { result } = renderHook(() => useReauthGate(methods, '/admin/system'));

    await act(async () => {
      result.current.run(reauthRequired, vi.fn(), undefined, undefined, {
        key: 'webhook.delete',
        params: 'webhook-1',
      });
      await Promise.resolve();
    });

    expect(JSON.parse(window.sessionStorage.getItem(storageKey)!)).toMatchObject({
      key: 'webhook.delete',
      params: 'webhook-1',
      path: '/admin/system',
    });
  });

  test('この画面に戻ってきたら、覚えておいた操作を一度だけ続け、知らせは出さない', () => {
    window.history.replaceState(null, '', '/admin/system?reauthenticated=oidc');
    stored('webhook.delete', '/admin/system', 'webhook-1');
    const onResume = vi.fn();

    const { unmount } = render(<Page onResume={onResume} />);

    expect(onResume).toHaveBeenCalledExactlyOnceWith('webhook-1');
    expect(window.sessionStorage.getItem(storageKey)).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();

    // 読み直しても、もう一度は続けない。
    unmount();
    render(<Page onResume={onResume} />);
    expect(onResume).toHaveBeenCalledOnce();
  });

  test.each([
    { name: 'IdP から戻ったのでない', url: '/admin/system', path: '/admin/system' },
    { name: 'ほかの画面で覚えた', url: '/admin/system?reauthenticated=oidc', path: '/admin/users' },
  ])('$nameときは、続けない', ({ url, path }) => {
    window.history.replaceState(null, '', url);
    stored('webhook.delete', path, 'webhook-1');
    const onResume = vi.fn();

    render(<Page onResume={onResume} />);

    expect(onResume).not.toHaveBeenCalled();
  });

  test('このページを読み込んだ後に覚えたものは、戻ってきた URL のままでも続けない', async () => {
    window.history.replaceState(null, '', '/admin/system?reauthenticated=oidc');
    const { result } = renderHook(() => useReauthGate(methods, '/admin/system'));
    await act(async () => {
      result.current.run(reauthRequired, vi.fn(), undefined, undefined, {
        key: 'webhook.delete',
        params: 'webhook-2',
      });
      await Promise.resolve();
    });
    const onResume = vi.fn();

    render(<Page onResume={onResume} />);

    expect(onResume).not.toHaveBeenCalled();
  });

  test('params を渡した続け方は、同じ params の操作だけを続ける (一覧の行ごと)', () => {
    window.history.replaceState(null, '', '/admin/system?reauthenticated=oidc');
    stored('webhook.delete', '/admin/system', 'webhook-2');
    const first = vi.fn();
    const second = vi.fn();
    function Row({ id, onResume }: { id: string; onResume: () => void }) {
      useReauthResume('webhook.delete', onResume, id);

      return null;
    }

    render(
      <>
        <Row id="webhook-1" onResume={first} />
        <Row id="webhook-2" onResume={second} />
      </>,
    );

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledOnce();
  });

  test('続け方のない操作を待っていたときは、もう一度操作するよう知らせる', () => {
    window.history.replaceState(null, '', '/admin/system?reauthenticated=oidc');
    stored(null);

    render(<Page onResume={vi.fn()} />);

    expect(screen.getByRole('status')).toBeTruthy();
  });

  test('再認証に失敗して戻ったときは、理由を出し、続けない', () => {
    window.history.replaceState(null, '', '/admin/system?error=oidc_denied');
    stored('webhook.delete', '/admin/system', 'webhook-1');
    const onResume = vi.fn();

    render(<Page onResume={onResume} />);

    expect(screen.getByRole('alert')).toBeTruthy();
    expect(onResume).not.toHaveBeenCalled();
    expect(window.sessionStorage.getItem(storageKey)).toBeNull();
  });
});

describe('入力を始める前の再認証 (#916)', () => {
  afterEach(() => {
    window.sessionStorage.clear();
    vi.restoreAllMocks();
  });

  test('重要な操作ができるなら、すぐに onReady を呼ぶ', async () => {
    vi.spyOn(authApi, 'reauthenticationStatus').mockResolvedValue({
      authenticatedAt: '2026-10-23T03:00:00Z',
      recent: true,
    });
    const { result } = renderHook(() => useReauthGate(methods, '/admin/system'));
    const onReady = vi.fn();

    await act(async () => {
      result.current.prepare({ key: 'webhook.create' }, onReady);
      await Promise.resolve();
    });

    expect(onReady).toHaveBeenCalledOnce();
    expect(window.sessionStorage.getItem(storageKey)).toBeNull();
  });

  test('再認証が要るなら、再認証の画面を出し、戻ってきたときのために覚えておく', async () => {
    window.history.replaceState(null, '', '/admin/system');
    vi.spyOn(authApi, 'reauthenticationStatus').mockResolvedValue({
      authenticatedAt: '2026-10-23T03:00:00Z',
      recent: false,
    });
    const { result } = renderHook(() => useReauthGate(methods, '/admin/system'));
    const onReady = vi.fn();

    await act(async () => {
      result.current.prepare({ key: 'webhook.create' }, onReady);
      await Promise.resolve();
    });

    expect(onReady).not.toHaveBeenCalled();
    render(<MantineProvider>{result.current.modal}</MantineProvider>);
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(JSON.parse(window.sessionStorage.getItem(storageKey)!)).toMatchObject({
      key: 'webhook.create',
    });
  });
});

// NameInput は、入力のある重要な操作の入力欄の代わり。
function NameInput({ prepare }: { prepare: ReturnType<typeof useReauthGate>['prepare'] }) {
  const input = useReauthBeforeInput(prepare, 'apiToken.name');

  return <input aria-label="name" ref={input.ref} onFocus={input.onFocus} />;
}

describe('useReauthBeforeInput (#916)', () => {
  afterEach(() => {
    window.sessionStorage.clear();
  });

  test('最初にフォーカスしたときだけ、入力を始める前の再認証を確かめる', () => {
    window.history.replaceState(null, '', '/settings/account');
    const prepare = vi.fn();
    render(<NameInput prepare={prepare} />);

    fireEvent.focus(screen.getByLabelText('name'));
    fireEvent.focus(screen.getByLabelText('name'));

    expect(prepare).toHaveBeenCalledExactlyOnceWith({ key: 'apiToken.name' }, expect.any(Function));
  });

  test('IdP での再認証から戻ってきたら、入力欄にフォーカスし直し、もう一度は確かめない', () => {
    window.history.replaceState(null, '', '/settings/account?reauthenticated=oidc');
    stored('apiToken.name', '/settings/account');
    const prepare = vi.fn();
    render(<NameInput prepare={prepare} />);

    expect(document.activeElement).toBe(screen.getByLabelText('name'));
    expect(prepare).not.toHaveBeenCalled();
  });
});
