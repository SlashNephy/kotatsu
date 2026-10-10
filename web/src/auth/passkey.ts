// パスキー (WebAuthn) で確かめる。WebAuthn は、Secure Context (HTTPS か localhost) でしか使えない (設計書 9.3)。
import { browserSupportsWebAuthn, startAuthentication } from '@simplewebauthn/browser';
import type { AuthenticationResponseJSON } from '@simplewebauthn/browser';

import { AuthError, type PasskeyAssertion } from './api';

// PasskeyCancelledError は、ユーザーがパスキーの確認を取り消したか、使えるパスキーがなかったことを表す。
// ブラウザーは、プライバシーのため、この 2 つを区別させない。
export class PasskeyCancelledError extends Error {
  constructor() {
    super('passkey ceremony was cancelled');
    this.name = 'PasskeyCancelledError';
  }
}

// passkeyAvailable は、この画面でパスキーを使えるかを返す。
export function passkeyAvailable(): boolean {
  return window.isSecureContext && browserSupportsWebAuthn();
}

// verifyWithPasskey は、begin で確認を始め、認証器に署名させ、confirm で確かめる。
export async function verifyWithPasskey<T>(
  begin: () => Promise<PasskeyAssertion>,
  confirm: (credential: AuthenticationResponseJSON) => Promise<T>,
): Promise<T> {
  // 使えない画面では、サーバーに手続きを始めさせない。
  if (!passkeyAvailable()) {
    throw new AuthError(400, 'PASSKEY_UNAVAILABLE', undefined, undefined);
  }
  const assertion = await begin();
  let credential: AuthenticationResponseJSON;
  try {
    credential = await startAuthentication({ optionsJSON: assertion.options });
  } catch (caught) {
    if (caught instanceof Error && caught.name === 'NotAllowedError') {
      throw new PasskeyCancelledError();
    }
    throw caught;
  }

  return confirm(credential);
}
