import { Alert, Anchor, Button, PinInput, Stack, Text, TextInput } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { AuthError, authApi } from '../../auth/api';
import { AuthCard } from '../../auth/AuthCard';
import { authErrorMessage } from '../../auth/messages';

export const Route = createFileRoute('/login/second-factor')({
  component: SecondFactor,
});

// SecondFactor は、認証アプリのコードか、リカバリーコードで、ログインを完了させる画面。
function SecondFactor() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [useRecovery, setUseRecovery] = useState(false);
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const submit = async (event?: FormEvent) => {
    event?.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await (useRecovery ? authApi.recoveryCode(code.trim()) : authApi.totp(code));
      // セッションの Cookie が入ったので、画面を読み直して、ログイン後のデータを取り直す。
      window.location.assign('/');
    } catch (caught) {
      if (caught instanceof AuthError && caught.code === 'LOGIN_REQUIRED') {
        await navigate({ to: '/login' });

        return;
      }
      setError(authErrorMessage(t, caught));
      setCode('');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AuthCard title={t('auth.secondFactor.title')}>
      <form onSubmit={(event) => void submit(event)}>
        <Stack>
          {useRecovery ? (
            <TextInput
              label={t('auth.secondFactor.recoveryCode')}
              value={code}
              onChange={(e) => setCode(e.currentTarget.value)}
              placeholder="xxxxx-xxxxx"
              autoComplete="off"
              required
            />
          ) : (
            <Stack gap={4}>
              <Text size="sm">{t('auth.secondFactor.totpDescription')}</Text>
              <PinInput
                aria-label={t('auth.secondFactor.totp')}
                length={6}
                type="number"
                oneTimeCode
                value={code}
                onChange={setCode}
                autoFocus
              />
            </Stack>
          )}
          {error !== null && (
            <Alert color="red" role="alert">
              {error}
            </Alert>
          )}
          <Button type="submit" loading={submitting} disabled={code === ''}>
            {t('auth.secondFactor.submit')}
          </Button>
          <Anchor
            component="button"
            type="button"
            size="sm"
            onClick={() => {
              setUseRecovery(!useRecovery);
              setCode('');
              setError(null);
            }}
          >
            {useRecovery ? t('auth.secondFactor.useTOTP') : t('auth.secondFactor.useRecoveryCode')}
          </Anchor>
        </Stack>
      </form>
    </AuthCard>
  );
}
