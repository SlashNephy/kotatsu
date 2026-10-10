import { Alert, Anchor, Button, Divider, PinInput, Stack, Text, TextInput } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { AuthError, authApi, type SecondFactor as Method } from '../../auth/api';
import { AuthCard } from '../../auth/AuthCard';
import { authErrorMessage } from '../../auth/messages';
import { passkeyAvailable, verifyWithPasskey } from '../../auth/passkey';

const allMethods: readonly Method[] = ['totp', 'passkey', 'recovery_code'];

type SecondFactorSearch = {
  // methods は、ユーザーが登録した第二要素。パスワードを確かめた応答から受け取る。
  methods?: Method[];
};

export const Route = createFileRoute('/login/second-factor')({
  validateSearch: (search: Record<string, unknown>): SecondFactorSearch => {
    const methods = search['methods'];
    if (!Array.isArray(methods)) {
      return {};
    }

    return { methods: allMethods.filter((method) => methods.includes(method)) };
  },
  component: SecondFactor,
});

// SecondFactor は、パスキー、認証アプリのコード、リカバリーコードのどれかで、ログインを完了させる画面。
function SecondFactor() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  // 画面を直接開いたときは、登録した第二要素が分からないので、コードの入力を出す。
  const { methods = ['totp', 'recovery_code'] } = Route.useSearch();
  const hasPasskey = methods.includes('passkey');
  const hasCode = methods.includes('totp') || methods.includes('recovery_code');
  const [useRecovery, setUseRecovery] = useState(false);
  const [code, setCode] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // complete は、verify でログインを完了させる。待っているログインが切れていれば、パスワードの入力に戻す。
  const complete = async (verify: () => Promise<unknown>) => {
    setSubmitting(true);
    setError(null);
    try {
      await verify();
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

  const submit = (event: FormEvent) => {
    event.preventDefault();
    void complete(() => (useRecovery ? authApi.recoveryCode(code.trim()) : authApi.totp(code)));
  };

  const verifyPasskey = () =>
    void complete(() =>
      verifyWithPasskey(authApi.beginPasskeySecondFactor, authApi.confirmPasskeySecondFactor),
    );

  return (
    <AuthCard title={t('auth.secondFactor.title')}>
      {hasPasskey && (
        <Stack gap="xs">
          <Text size="sm">{t('auth.secondFactor.passkeyDescription')}</Text>
          <Button loading={submitting} disabled={!passkeyAvailable()} onClick={verifyPasskey}>
            {t('auth.secondFactor.passkey')}
          </Button>
          {!passkeyAvailable() && (
            <Text size="xs" c="dimmed">
              {t('auth.login.passkeyUnavailable')}
            </Text>
          )}
        </Stack>
      )}
      {hasPasskey && hasCode && <Divider label={t('auth.secondFactor.or')} />}
      {error !== null && !hasCode && (
        <Alert color="red" role="alert">
          {error}
        </Alert>
      )}
      {hasCode && (
        <form onSubmit={submit}>
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
                  autoFocus={!hasPasskey}
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
              {useRecovery
                ? t('auth.secondFactor.useTOTP')
                : t('auth.secondFactor.useRecoveryCode')}
            </Anchor>
          </Stack>
        </form>
      )}
    </AuthCard>
  );
}
