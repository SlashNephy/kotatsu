import type { TFunction } from 'i18next';

import { authErrorMessage } from '../auth/messages';
import { graphQLErrorDetails } from '../relay/environment';

const passwordReasons = ['too_short', 'too_long', 'blocklisted', 'breached', 'too_weak'] as const;
const graphQLCodes = ['BAD_USER_INPUT', 'NOT_FOUND', 'FORBIDDEN', 'CONFLICT'] as const;

function isOneOf<T extends string>(values: readonly T[], value: string | undefined): value is T {
  return value !== undefined && (values as readonly string[]).includes(value);
}

// settingsErrorMessage は、設定の画面での操作のエラーを、画面に出す文に直す。
// GraphQL のエラーはコードで、認証の API のエラーは authErrorMessage で直す。
export function settingsErrorMessage(t: TFunction, error: unknown): string {
  const { code, reason } = graphQLErrorDetails(error);
  if (code === 'BAD_USER_INPUT' && isOneOf(passwordReasons, reason)) {
    return t(`password.rejected.${reason}`);
  }
  if (code === 'UNAUTHENTICATED') {
    return t('auth.errors.UNAUTHENTICATED');
  }
  if (isOneOf(graphQLCodes, code)) {
    return t(`settings.errors.${code}`);
  }

  return authErrorMessage(t, error);
}
