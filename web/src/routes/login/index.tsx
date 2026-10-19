import { Alert, Button, Divider, PasswordInput, Stack, Text, TextInput } from '@mantine/core';
import { LogIn, ScanFace } from 'lucide-react';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { authApi, type OIDCStatus } from '../../auth/api';
import { AuthCard } from '../../auth/AuthCard';
import { authErrorMessage, oidcErrorMessage } from '../../auth/messages';
import { passkeyAvailable, verifyWithPasskey } from '../../auth/passkey';

type LoginSearch = {
  // setup は、初回セットアップの直後かどうか。
  setup?: boolean;
  // error は、OIDC でのログインに失敗して戻ってきたときの理由 (oidc_denied など)。
  error?: string;
};

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };

export const Route = createFileRoute('/login/')({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    ...(search['setup'] === true ? { setup: true } : {}),
    ...(typeof search['error'] === 'string' && search['error'] !== ''
      ? { error: search['error'] }
      : {}),
  }),
  // OIDC のボタンを出すかを確かめる。確かめられなくても、パスワードとパスキーではログインできる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: Login,
});

// Login は、ユーザー名とパスワードを確かめる画面。続けて、第二要素を確かめるか、登録する (設計書 9.3)。
function Login() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { setup, error: oidcError } = Route.useSearch();
  const oidc = Route.useLoaderData();
  const canUsePasskey = passkeyAvailable();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  // passkeyError は、パスキーでのログインのエラー。パスワードの入力欄のエラーと分けて、ボタンの近くに出す。
  const [passkeyError, setPasskeyError] = useState<string | null>(null);
  // oidcBeginError は、IdP へ送り出せなかったときのエラー。OIDC のボタンの近くに出す。
  const [oidcBeginError, setOIDCBeginError] = useState<string | null>(null);
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
      window.location.assign(import.meta.env.BASE_URL);
    } catch (caught) {
      setPasskeyError(authErrorMessage(t, caught));
    } finally {
      setSubmitting(false);
    }
  };

  // IdP へ送り出す。戻ってくると、セッションの Cookie を受け取ってホームに進むか、理由を付けてこの画面に戻る。
  const loginWithOIDC = async () => {
    setSubmitting(true);
    setError(null);
    setPasskeyError(null);
    setOIDCBeginError(null);
    try {
      const { authorizationUrl } = await authApi.beginOIDC();
      window.location.assign(authorizationUrl);
    } catch (caught) {
      setOIDCBeginError(authErrorMessage(t, caught));
      setSubmitting(false);
    }
  };

  return (
    <AuthCard title={t('auth.login.title')}>
      {setup === true && <Alert color="mikan">{t('auth.setup.done')}</Alert>}
      {oidcError !== undefined && (
        <Alert color="red" role="alert">
          {oidcErrorMessage(t, oidcError)}
        </Alert>
      )}
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
          <Button type="submit" loading={submitting} leftSection={<LogIn size={16} aria-hidden />}>
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
          leftSection={<ScanFace size={16} aria-hidden />}
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
      {oidc.enabled && (
        <Stack gap="xs">
          <Button
            variant="default"
            loading={submitting}
            onClick={() => void loginWithOIDC()}
            leftSection={<LogIn size={16} aria-hidden />}
          >
            {t('auth.login.oidc', { name: oidc.displayName })}
          </Button>
          {oidcBeginError !== null && (
            <Alert color="red" role="alert">
              {oidcBeginError}
            </Alert>
          )}
        </Stack>
      )}
    </AuthCard>
  );
}
