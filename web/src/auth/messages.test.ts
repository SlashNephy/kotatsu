import { describe, expect, test } from 'vitest';

import i18next from '../i18n';
import { AuthError } from './api';
import { authErrorMessage } from './messages';

describe('authErrorMessage', () => {
  test.each([
    {
      name: 'サーバーが拒否したパスワードの理由',
      error: new AuthError(400, 'WEAK_PASSWORD', 'blocklisted', undefined),
      want: 'よく使われているパスワードなので使えません',
    },
    {
      name: '試行回数の制限の待ち時間',
      error: new AuthError(429, 'TOO_MANY_ATTEMPTS', undefined, 4),
      want: '試行が多すぎます。4 秒ほど待ってからやり直してください',
    },
    {
      name: '知っているエラーコード',
      error: new AuthError(401, 'INVALID_CREDENTIALS', undefined, undefined),
      want: 'ユーザー名かパスワードが違います',
    },
    {
      name: '知らないエラーコード',
      error: new AuthError(500, 'INTERNAL', undefined, undefined),
      want: 'うまくいきませんでした。時間をおいてやり直してください',
    },
    {
      name: '知らない理由',
      error: new AuthError(400, 'WEAK_PASSWORD', 'unknown', undefined),
      want: 'うまくいきませんでした。時間をおいてやり直してください',
    },
    {
      name: '通信のエラー',
      error: new TypeError('Failed to fetch'),
      want: 'うまくいきませんでした。時間をおいてやり直してください',
    },
  ])('$name', ({ error, want }) => {
    expect(authErrorMessage(i18next.t, error)).toBe(want);
  });
});
