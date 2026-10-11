import { describe, expect, test } from 'vitest';

import { AuthError } from '../auth/api';
import i18next from '../i18n';
import { GraphQLRequestError } from '../relay/environment';
import { settingsErrorMessage } from './errors';

// mutationError は、mutate が GraphQL のエラーで失敗するときと同じ形のエラーを作る。
function mutationError(code: string, reason?: string) {
  return Object.assign(new Error(code), {
    source: { errors: [{ message: code, extensions: { code, reason } }] },
  });
}

describe('settingsErrorMessage', () => {
  test.each([
    {
      name: 'パスワードの規定に当たった理由',
      error: mutationError('BAD_USER_INPUT', 'too_weak'),
      want: '推測されやすいパスワードなので使えません。長くするか、単語を組み合わせてください',
    },
    {
      name: '理由のない入力の誤り',
      error: mutationError('BAD_USER_INPUT'),
      want: '入力した内容を受け付けられませんでした。値を確かめてください',
    },
    {
      name: '管理者でない',
      error: mutationError('FORBIDDEN'),
      want: 'この操作は管理者だけができます',
    },
    {
      name: '問い合わせのエラーのコード',
      error: new GraphQLRequestError('login required', 401, 'UNAUTHENTICATED'),
      want: 'ログインしていません。もう一度ログインしてください',
    },
    {
      name: '認証の API のエラー',
      error: new AuthError(401, 'INVALID_SECOND_FACTOR', undefined, undefined),
      want: 'コードが違います',
    },
    {
      name: '知らないエラー',
      error: mutationError('INTERNAL'),
      want: 'うまくいきませんでした。時間をおいてやり直してください',
    },
  ])('$name', ({ error, want }) => {
    expect(settingsErrorMessage(i18next.t, error)).toBe(want);
  });
});
