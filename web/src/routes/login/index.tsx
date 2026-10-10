import { Alert, Button, PasswordInput, Stack, TextInput } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { authApi } from '../../auth/api';
import { AuthCard } from '../../auth/AuthCard';
import { authErrorMessage } from '../../auth/messages';

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
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const result = await authApi.login(username, password);
      await navigate({
        to: result.next === 'enroll_second_factor' ? '/login/enroll' : '/login/second-factor',
      });
    } catch (caught) {
      setError(authErrorMessage(t, caught));
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
    </AuthCard>
  );
}
