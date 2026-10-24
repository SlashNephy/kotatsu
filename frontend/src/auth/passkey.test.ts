import type { AuthenticationResponseJSON } from '@simplewebauthn/browser';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';

import { AuthError, type PasskeyAssertion } from './api';
import { PasskeyCancelledError, passkeyAvailable, verifyWithPasskey } from './passkey';

const browser = vi.hoisted(() => ({
  browserSupportsWebAuthn: vi.fn<() => boolean>(),
  startAuthentication: vi.fn<() => Promise<AuthenticationResponseJSON>>(),
}));
vi.mock('@simplewebauthn/browser', () => browser);

const assertion: PasskeyAssertion = {
  options: { challenge: 'Y2hhbGxlbmdl', rpId: 'localhost' },
  expiresAt: '2026-10-10T00:05:00Z',
};
const credential = { id: 'cred', rawId: 'cred' } as AuthenticationResponseJSON;

function setSecureContext(secure: boolean) {
  Object.defineProperty(window, 'isSecureContext', { configurable: true, value: secure });
}

describe('passkeyAvailable', () => {
  test.each([
    { name: 'Secure Context で WebAuthn がある', secure: true, supported: true, want: true },
    { name: 'Secure Context でない', secure: false, supported: true, want: false },
    { name: 'WebAuthn がない', secure: true, supported: false, want: false },
  ])('$name', ({ secure, supported, want }) => {
    setSecureContext(secure);
    browser.browserSupportsWebAuthn.mockReturnValue(supported);
    expect(passkeyAvailable()).toBe(want);
  });
});

describe('verifyWithPasskey', () => {
  beforeEach(() => {
    setSecureContext(true);
    browser.browserSupportsWebAuthn.mockReturnValue(true);
  });
  afterEach(() => {
    vi.resetAllMocks();
  });

  test('認証器の応答を確かめに送る', async () => {
    browser.startAuthentication.mockResolvedValue(credential);
    const confirm = vi.fn().mockResolvedValue({ recoveryCodes: [] });

    await expect(verifyWithPasskey(() => Promise.resolve(assertion), confirm)).resolves.toEqual({
      recoveryCodes: [],
    });
    expect(browser.startAuthentication).toHaveBeenCalledWith({ optionsJSON: assertion.options });
    expect(confirm).toHaveBeenCalledWith(credential);
  });

  test('使えない画面では、手続きを始めない', async () => {
    setSecureContext(false);
    const begin = vi.fn();

    await expect(verifyWithPasskey(begin, vi.fn())).rejects.toMatchObject(
      new AuthError(400, 'PASSKEY_UNAVAILABLE', undefined, undefined),
    );
    expect(begin).not.toHaveBeenCalled();
  });

  test.each([
    {
      name: '取り消しは PasskeyCancelledError にする',
      error: Object.assign(new Error('cancelled'), { name: 'NotAllowedError' }),
      want: PasskeyCancelledError,
    },
    {
      name: 'ほかのエラーはそのまま投げる',
      error: Object.assign(new Error('bad'), { name: 'SecurityError' }),
      want: Error,
    },
  ])('$name', async ({ error, want }) => {
    browser.startAuthentication.mockRejectedValue(error);
    const confirm = vi.fn();

    const result = verifyWithPasskey(() => Promise.resolve(assertion), confirm);
    await expect(result).rejects.toBeInstanceOf(want);
    await expect(result).rejects.toHaveProperty(
      'name',
      want === Error ? 'SecurityError' : 'PasskeyCancelledError',
    );
    expect(confirm).not.toHaveBeenCalled();
  });
});
