import type { TFunction } from 'i18next';

import { AuthError } from './api';
import { PasskeyCancelledError } from './passkey';

const knownCodes = [
  'INVALID_CREDENTIALS',
  'TOO_MANY_ATTEMPTS',
  'INVALID_SECOND_FACTOR',
  'LOGIN_REQUIRED',
  'INVALID_SETUP_TOKEN',
  'SETUP_DONE',
  'SECOND_FACTOR_ENROLLED',
  'INVALID_PASSKEY',
  'PASSKEY_CEREMONY_EXPIRED',
  'PASSKEY_UNAVAILABLE',
  'NO_PASSKEY',
  'OIDC_UNAVAILABLE',
] as const;

const knownReasons = ['too_short', 'too_long', 'blocklisted', 'breached', 'too_weak'] as const;

function isKnown<T extends string>(values: readonly T[], value: string | undefined): value is T {
  return value !== undefined && (values as readonly string[]).includes(value);
}

// authErrorMessage は、認証の API のエラーを、画面に出す文に直す。
// パスワードの規定に当たったときは、サーバーが返した理由を出す (#59 の完了条件)。
export function authErrorMessage(t: TFunction, error: unknown): string {
  if (error instanceof PasskeyCancelledError) {
    return t('auth.errors.PASSKEY_CANCELLED');
  }
  if (!(error instanceof AuthError)) {
    return t('auth.errors.unknown');
  }
  if (error.code === 'WEAK_PASSWORD' && isKnown(knownReasons, error.reason)) {
    return t(`password.rejected.${error.reason}`);
  }
  if (error.code === 'TOO_MANY_ATTEMPTS') {
    return t('auth.errors.TOO_MANY_ATTEMPTS', { seconds: error.retryAfter ?? 1 });
  }
  if (isKnown(knownCodes, error.code)) {
    return t(`auth.errors.${error.code}`);
  }

  return t('auth.errors.unknown');
}

const oidcReasons = [
  'oidc_expired',
  'oidc_invalid',
  'oidc_denied',
  'oidc_username_taken',
  'oidc_unavailable',
] as const;

// oidcErrorMessage は、OIDC でのログインに失敗して戻ってきた理由を、画面に出す文に直す。
export function oidcErrorMessage(t: TFunction, reason: string): string {
  if (isKnown(oidcReasons, reason)) {
    return t(`auth.oidcErrors.${reason}`);
  }

  return t('auth.errors.unknown');
}
