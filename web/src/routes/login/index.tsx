import { Alert, Button, Divider, PasswordInput, Stack, Text, TextInput } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { authApi } from '../../auth/api';
import { AuthCard } from '../../auth/AuthCard';
import { authErrorMessage } from '../../auth/messages';
import { passkeyAvailable, verifyWithPasskey } from '../../auth/passkey';

type LoginSearch = {
  // setup は、初回セットアップの直後かどうか。
  setup?: boolean;
};

export const Route = createFileRoute('/login/')({
  validateSearch: (search: Record<string, unknown>): LoginSearch =>
    search['setup'] === true ? { setup: true } : {},
  component: Login,
});

// Login は、ユーザー名とパスワードを確かめる画面。続けて、第二要素を確かめるか、登録する (設計書 9.3)。
function Login() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { setup } = Route.useSearch();
  const canUsePasskey = passkeyAvailable();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  // passkeyError は、パスキーでのログインのエラー。パスワードの入力欄のエラーと分けて、ボタンの近くに出す。
  const [passkeyError, setPasskeyError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    setPasskeyError(null);
    try {
      const result = await authApi.login(username, password);
      await navigate(
        result.next === 'enroll_second_factor'
          ? { to: '/login/enroll' }
          : {
              to: '/login/second-factor',
              search: { methods: result.methods },
            },
      );
    } catch (caught) {
      setError(authErrorMessage(t, caught));
    } finally {
      setSubmitting(false);
    }
  };

  // パスキーは、端末を持っていることと、生体認証か PIN の両方を確かめるので、それだけでログインできる (設計書 9.3)。
  const loginWithPasskey = async () => {
    setSubmitting(true);
    setError(null);
    setPasskeyError(null);
    try {
      await verifyWithPasskey(authApi.beginPasskeyLogin, authApi.confirmPasskeyLogin);
      // セッションの Cookie が入ったので、画面を読み直して、ログイン後のデータを取り直す。
      window.location.assign('/');
    } catch (caught) {
      setPasskeyError(authErrorMessage(t, caught));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthCard title={t('auth.login.title')}>
      {setup === true && <Alert color="mikan">{t('auth.setup.done')}</Alert>}
      <form onSubmit={(event) => void submit(event)}>
        <Stack>
          <TextInput
            label={t('auth.username')}
            value={username}
            onChange={(e) => setUsername(e.currentTarget.value)}
            required
            autoComplete="username"
          />
          <PasswordInput
            label={t('auth.password')}
            value={password}
            onChange={(e) => setPassword(e.currentTarget.value)}
            required
            autoComplete="current-password"
          />
          {error !== null && (
            <Alert color="red" role="alert">
              {error}
            </Alert>
          )}
          <Button type="submit" loading={submitting}>
            {t('auth.login.submit')}
          </Button>
        </Stack>
      </form>
      <Divider label={t('auth.login.or')} />
      <Stack gap="xs">
        <Button
          variant="light"
          loading={submitting}
          disabled={!canUsePasskey}
          onClick={() => void loginWithPasskey()}
        >
          {t('auth.login.passkey')}
        </Button>
        {!canUsePasskey && (
          <Text size="xs" c="dimmed">
            {t('auth.login.passkeyUnavailable')}
          </Text>
        )}
        {passkeyError !== null && (
          <Alert color="red" role="alert">
            {passkeyError}
          </Alert>
        )}
      </Stack>
    </AuthCard>
  );
}
