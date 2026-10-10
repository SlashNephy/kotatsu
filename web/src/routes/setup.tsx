import { Alert, Button, PasswordInput, Stack, TextInput } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { authApi } from '../auth/api';
import { AuthCard } from '../auth/AuthCard';
import { authErrorMessage } from '../auth/messages';
import { PasswordStrength } from '../futon';

export const Route = createFileRoute('/setup')({
  component: Setup,
});

// Setup は、最初の管理者を作る画面 (設計書 9.2)。
function Setup() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [token, setToken] = useState('');
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await authApi.setup({ token: token.trim(), username, displayName, password });
      await navigate({ to: '/login', search: { setup: true } });
    } catch (caught) {
      setError(authErrorMessage(t, caught));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthCard title={t('auth.setup.title')} description={t('auth.setup.description')}>
      <form onSubmit={(event) => void submit(event)}>
        <Stack>
          <TextInput
            label={t('auth.setup.token')}
            value={token}
            onChange={(e) => setToken(e.currentTarget.value)}
            required
            autoComplete="off"
          />
          <TextInput
            label={t('auth.username')}
            value={username}
            onChange={(e) => setUsername(e.currentTarget.value)}
            required
            autoComplete="username"
          />
          <TextInput
            label={t('auth.displayName')}
            value={displayName}
            onChange={(e) => setDisplayName(e.currentTarget.value)}
            required
          />
          <PasswordInput
            label={t('auth.password')}
            value={password}
            onChange={(e) => setPassword(e.currentTarget.value)}
            required
            autoComplete="new-password"
          />
          <PasswordStrength password={password} userInputs={[username, displayName]} />
          {error !== null && (
            <Alert color="red" role="alert">
              {error}
            </Alert>
          )}
          <Button type="submit" loading={submitting}>
            {t('auth.setup.submit')}
          </Button>
        </Stack>
      </form>
    </AuthCard>
  );
}
