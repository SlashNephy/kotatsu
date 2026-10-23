import { Alert, Button, Checkbox, Group, Stack, Text } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { authApi, type OIDCStatus } from '../auth/api';
import { AuthCard } from '../auth/AuthCard';
import { rememberReturnTo } from '../auth/returnTo';
import { ErrorBoundary, PageLoader } from '../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../relay/environment';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { settingsErrorMessage } from '../settings/errors';
import { mutate } from '../settings/mutate';
import { clientHost } from '../settings/OAuthGrants';
import {
  reauthMethods,
  useReauthBeforeInput,
  useReauthGate,
  useReauthResume,
} from '../settings/useReauthGate';
import type { authorizeApproveMutation } from './__generated__/authorizeApproveMutation.graphql';
import type { authorizeDenyMutation } from './__generated__/authorizeDenyMutation.graphql';
import type { authorizeQuery } from './__generated__/authorizeQuery.graphql';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };

// 同意の画面 (#965、設計書 9.9)。認可のエンドポイント (/oauth/authorize) が、受け取ったクエリをそのまま付けて、ここへ送る。
// クエリは、ログインした後に GraphQL で確かめる。ログインしていなければ、ログインさせてから戻る。
export const Route = createFileRoute('/authorize')({
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: AuthorizePage,
});

// currentPath は、今の画面のパスとクエリ。ログインと再認証を終えたら、ここへ戻す。
function currentPath(): string {
  return window.location.pathname + window.location.search;
}

function AuthorizePage() {
  const { t } = useTranslation();
  // クエリは、認可のエンドポイントに届いたものを、書き換えずにそのまま確かめる。
  const [query] = useState(() => window.location.search.replace(/^\?/, ''));

  return (
    <ErrorBoundary fallback={(error) => <AuthorizeError error={error} />}>
      <Suspense fallback={<PageLoader />}>
        <AuthCard title={t('oauth.title')}>
          <Consent query={query} />
        </AuthCard>
      </Suspense>
    </ErrorBoundary>
  );
}

// AuthorizeError は、認可のリクエストを確かめられなかったときの画面。ログインしていなければ、ログインの画面へ移る。
function AuthorizeError({ error }: { error: unknown }) {
  const { t } = useTranslation();
  const unauthenticated = isUnauthenticated(error);
  useEffect(() => {
    if (unauthenticated) {
      rememberReturnTo(currentPath());
      window.location.assign(`${import.meta.env.BASE_URL}login`);
    }
  }, [unauthenticated]);
  if (unauthenticated) {
    return <PageLoader />;
  }
  const { code } = graphQLErrorDetails(error);

  return (
    <AuthCard title={t('oauth.title')}>
      <Alert color="red" role="alert">
        {code === 'NOT_FOUND'
          ? t('oauth.disabled')
          : code === 'BAD_USER_INPUT'
            ? t('oauth.invalidRequest')
            : t('viewer.error')}
      </Alert>
    </AuthCard>
  );
}

function Consent({ query }: { query: string }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  const data = useLazyLoadQuery<authorizeQuery>(
    graphql`
      query authorizeQuery($query: String!) {
        oauthAuthorization(query: $query) {
          authorization {
            clientId
            clientName
            clientUri
            redirectUri
            scopes
            allowedScopes
          }
          errorRedirectUrl
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
    { query },
    { fetchPolicy: 'network-only' },
  );
  const gate = useReauthGate(reauthMethods(data.secondFactors, data.passkeys, oidc), currentPath());
  const { authorization, errorRedirectUrl } = data.oauthAuthorization;
  // 初めは、求められたスコープのうち、ロールで許すものをすべて選んでおく。
  const [chosen, setChosen] = useState<string[]>(
    () =>
      authorization?.scopes.filter((scope) => authorization.allowedScopes.includes(scope)) ?? [],
  );
  const [error, setError] = useState<string | null>(null);
  const [leaving, setLeaving] = useState(false);

  const leave = (url: string) => {
    setLeaving(true);
    window.location.assign(url);
  };
  const fail = (caught: unknown) => setError(settingsErrorMessage(t, caught));
  // 同意と拒否は、IdP で再認証して戻ってきたら、そのまま続ける (#916)。選んだスコープは秘密ではないので、覚えておく。
  const approve = (scopes: readonly string[]) =>
    gate.run(
      async () => {
        const response = await mutate<authorizeApproveMutation>(
          environment,
          graphql`
            mutation authorizeApproveMutation($query: String!, $scopes: [String!]!) {
              approveOAuthAuthorization(query: $query, scopes: $scopes)
            }
          `,
          { query, scopes: [...scopes] },
        );
        leave(response.approveOAuthAuthorization);
      },
      fail,
      undefined,
      undefined,
      { key: 'oauth.approve', params: scopes.join(' ') },
    );
  const deny = () =>
    gate.run(
      async () => {
        const response = await mutate<authorizeDenyMutation>(
          environment,
          graphql`
            mutation authorizeDenyMutation($query: String!) {
              denyOAuthAuthorization(query: $query)
            }
          `,
          { query },
        );
        leave(response.denyOAuthAuthorization);
      },
      fail,
      undefined,
      undefined,
      { key: 'oauth.deny' },
    );
  useReauthResume('oauth.approve', (params) =>
    approve((params ?? '').split(' ').filter((scope) => scope !== '')),
  );
  useReauthResume('oauth.deny', deny);
  // スコープを選び直す前に再認証させる。IdP で再認証すると、ページを離れ、選び直したものが消えるため。
  const scopesInput = useReauthBeforeInput(gate.prepare, 'oauth.scopes');

  // パラメーターが正しくなければ、エラーを付けてクライアントへ戻す。
  useEffect(() => {
    if (errorRedirectUrl != null) {
      window.location.assign(errorRedirectUrl);
    }
  }, [errorRedirectUrl]);
  if (authorization == null) {
    // クライアントへ戻すまでは読み込み中にする。戻す先もなければ、エラーを出す。
    return errorRedirectUrl != null ? (
      <PageLoader />
    ) : (
      <Alert color="red" role="alert">
        {t('oauth.invalidRequest')}
      </Alert>
    );
  }

  return (
    <Stack>
      {gate.modal}
      <Stack gap={2}>
        <Text fw={700} size="lg">
          {authorization.clientName}
        </Text>
        <Text size="xs" c="dimmed">
          {t('oauth.client', { host: clientHost(authorization.clientId) })}
        </Text>
        <Text size="xs" c="dimmed">
          {t('oauth.redirect', { host: clientHost(authorization.redirectUri) })}
        </Text>
      </Stack>
      <Text size="sm">{t('oauth.description', { name: authorization.clientName })}</Text>
      <Stack gap="xs" onFocus={scopesInput.onFocus}>
        {authorization.scopes.map((scope, index) => {
          const allowed = authorization.allowedScopes.includes(scope);

          return (
            <Checkbox
              key={scope}
              ref={index === 0 ? scopesInput.ref : undefined}
              label={t(scope === 'mcp:write' ? 'oauth.scopeWrite' : 'oauth.scopeRead')}
              description={
                allowed
                  ? t(scope === 'mcp:write' ? 'oauth.scopeWriteHelp' : 'oauth.scopeReadHelp')
                  : t('oauth.scopeNotAllowed')
              }
              disabled={!allowed}
              checked={chosen.includes(scope)}
              onChange={(event) => {
                const checked = event.currentTarget.checked;
                setChosen((current) =>
                  checked ? [...current, scope] : current.filter((value) => value !== scope),
                );
              }}
            />
          );
        })}
      </Stack>
      {error !== null && (
        <Alert color="red" role="alert">
          {error}
        </Alert>
      )}
      <Group justify="flex-end">
        <Button variant="default" onClick={deny} disabled={gate.busy || leaving}>
          {t('oauth.deny')}
        </Button>
        <Button
          onClick={() => approve(chosen)}
          disabled={chosen.length === 0 || gate.busy || leaving}
        >
          {t('oauth.approve')}
        </Button>
      </Group>
    </Stack>
  );
}
