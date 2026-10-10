import { Alert, Anchor, Button, PasswordInput, Stack, Text } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { authApi, type OIDCStatus } from '../../auth/api';
import { AuthCard } from '../../auth/AuthCard';
import { oidcErrorMessage } from '../../auth/messages';
import { Reauthenticate } from '../../auth/Reauthenticate';
import { ErrorBoundary, PasswordStrength } from '../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { passwordChangeMutation } from './__generated__/passwordChangeMutation.graphql';
import type { passwordViewerQuery } from './__generated__/passwordViewerQuery.graphql';

type PasswordSearch = {
  // error は、IdP での再認証に失敗して戻ってきたときの理由 (oidc_denied など)。
  error?: string;
};

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '' };

const passwordReasons = ['too_short', 'too_long', 'blocklisted', 'breached', 'too_weak'] as const;

const path = '/settings/password';

export const Route = createFileRoute('/settings/password')({
  validateSearch: (search: Record<string, unknown>): PasswordSearch =>
    typeof search['error'] === 'string' && search['error'] !== '' ? { error: search['error'] } : {},
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: ChangePasswordPage,
});

// ChangePasswordPage は、ログイン中のユーザーのパスワードを変える画面 (設計書 9.3)。
// パスワードを持たないユーザー (OIDC やパスキーだけで使う) なら、パスワードを設定する。
function ChangePasswordPage() {
  const { t } = useTranslation();

  return (
    <AuthCard title={t('settings.password.title')}>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('settings.password.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <ChangePassword />
        </Suspense>
      </ErrorBoundary>
    </AuthCard>
  );
}

function ChangePassword() {
  const { t } = useTranslation();
  const { error: oidcError } = Route.useSearch();
  const oidc = Route.useLoaderData();
  const data = useLazyLoadQuery<passwordViewerQuery>(
    graphql`
      query passwordViewerQuery {
        viewer {
          username
          displayName
        }
        secondFactors {
          totp
          remainingRecoveryCodes
        }
        passkeys {
          id
        }
      }
    `,
    {},
    // 再認証の後に、登録した第二要素が変わっていることがあるので、毎回取り直す。
    { fetchPolicy: 'network-only' },
  );
  const [commit, inFlight] = useMutation<passwordChangeMutation>(graphql`
    mutation passwordChangeMutation($password: String!) {
      changePassword(password: $password)
    }
  `);
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(
    oidcError === undefined ? null : oidcErrorMessage(t, oidcError),
  );
  const [done, setDone] = useState(false);
  // needsReauthentication は、直近の認証から時間が経っていて、もう一度確かめる必要があるかどうか。
  const [needsReauthentication, setNeedsReauthentication] = useState(false);

  const change = () => {
    setError(null);
    setDone(false);
    commit({
      variables: { password },
      onCompleted: () => {
        setDone(true);
        setPassword('');
      },
      onError: (caught) => {
        const details = graphQLErrorDetails(caught);
        if (details.code === 'REAUTHENTICATION_REQUIRED') {
          setNeedsReauthentication(true);

          return;
        }
        const reason = passwordReasons.find((known) => known === details.reason);
        if (details.code === 'BAD_USER_INPUT' && reason !== undefined) {
          setError(t(`password.rejected.${reason}`));
        } else if (details.code === 'UNAUTHENTICATED') {
          // 画面を開いている間に、セッションが切れた。
          setError(t('auth.errors.UNAUTHENTICATED'));
        } else {
          setError(t('auth.errors.unknown'));
        }
      },
    });
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    change();
  };

  if (needsReauthentication) {
    return (
      <Stack>
        <Reauthenticate
          methods={{
            totp: data.secondFactors.totp,
            recoveryCode: data.secondFactors.remainingRecoveryCodes > 0,
            passkey: data.passkeys.length > 0,
            oidc,
          }}
          returnTo={path}
          // 再認証を終えたら、入力済みのパスワードで、もう一度変更を試みる。
          onDone={() => {
            setNeedsReauthentication(false);
            change();
          }}
        />
        <Anchor
          component="button"
          type="button"
          size="sm"
          onClick={() => setNeedsReauthentication(false)}
        >
          {t('settings.password.cancel')}
        </Anchor>
      </Stack>
    );
  }

  return (
    <form onSubmit={submit}>
      <Stack>
        <Text size="sm" c="dimmed">
          {t('settings.password.description')}
        </Text>
        {/* パスワードマネージャーが、どのアカウントのパスワードかを判断できるよう、ユーザー名を渡す。 */}
        <input
          type="text"
          name="username"
          autoComplete="username"
          value={data.viewer.username}
          readOnly
          hidden
        />
        <PasswordInput
          label={t('settings.password.new')}
          value={password}
          onChange={(e) => setPassword(e.currentTarget.value)}
          required
          autoComplete="new-password"
        />
        <PasswordStrength
          password={password}
          userInputs={[data.viewer.username, data.viewer.displayName]}
        />
        {error !== null && (
          <Alert color="red" role="alert">
            {error}
          </Alert>
        )}
        {done && (
          <Alert color="green" role="status">
            {t('settings.password.done')}
          </Alert>
        )}
        <Button type="submit" loading={inFlight} disabled={password === ''}>
          {t('settings.password.submit')}
        </Button>
        <Anchor component={Link} to="/" size="sm">
          {t('notFound.back')}
        </Anchor>
      </Stack>
    </form>
  );
}
