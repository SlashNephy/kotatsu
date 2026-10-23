import { Alert, Anchor, Button, PasswordInput, Stack, Text } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { authApi, type OIDCStatus } from '../../auth/api';
import { AuthCard } from '../../auth/AuthCard';
import { oidcErrorMessage } from '../../auth/messages';
import { Reauthenticate } from '../../auth/Reauthenticate';
import { ErrorBoundary, PageLoader, PasswordStrength } from '../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { passwordChangeMutation } from './__generated__/passwordChangeMutation.graphql';
import type { passwordViewerQuery } from './__generated__/passwordViewerQuery.graphql';

type PasswordSearch = {
  // error は、IdP での再認証に失敗して戻ってきたときの理由 (oidc_denied など)。
  error?: string;
};

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };

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
        <Suspense fallback={<PageLoader />}>
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
    // 再認証の後に、登録した第二要素が変わっていることがあるので、毎回取り直す。前に読んだものがあれば、それを出しながら取り直す。
    { fetchPolicy: 'store-and-network' },
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
  // ほかの重要な操作のある画面の SudoGate と同じく、開いたときに、必要なら再認証させる (#981)。
  // IdP で再認証すると、ページを離れ、入れたパスワードが消えるため。この画面は全体がカードなので、カードを重ねる
  // SudoGate ではなく、フォームの代わりに再認証の画面を出す。
  // 確かめられなくても、入力は始めさせる。必要なら、変えるときに再認証を求められる。
  useEffect(() => {
    let active = true;
    authApi.reauthenticationStatus().then(
      ({ recent }) => {
        if (active && !recent) {
          setNeedsReauthentication(true);
        }
      },
      () => undefined,
    );

    return () => {
      active = false;
    };
  }, []);

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
          // 再認証を終えたら、入力済みのパスワードで、もう一度変更を試みる。入れる前に確かめたときは、入力に進む。
          onDone={() => {
            setNeedsReauthentication(false);
            if (password !== '') {
              change();
            }
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
