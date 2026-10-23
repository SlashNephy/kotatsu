import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Fieldset,
  Group,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { useDataChanges } from '../../changes/DataChanges';
import { ErrorBoundary, PageLoader, zonedFormat } from '../../futon';
import { isUnauthenticated } from '../../relay/environment';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';
import { deviceName } from '../../settings/device';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import { OAuthGrants } from '../../settings/OAuthGrants';
import type { sessionsQuery } from './__generated__/sessionsQuery.graphql';
import type { sessionsRevokeOthersMutation } from './__generated__/sessionsRevokeOthersMutation.graphql';
import type { sessionsRevokeMutation } from './__generated__/sessionsRevokeMutation.graphql';

export const Route = createFileRoute('/settings/sessions')({
  component: SessionsPage,
});

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// SessionsPage は、このアカウントを使っているもの (ログイン中の端末と、連携中のアプリ) を一覧し、切る画面
// (設計書 9.5、10.3、#980)。ほかの端末をログアウトさせるのも、連携を取り消すのも、乗っ取られた後の被害を抑える操作なので、
// 再認証を求めない。セキュリティの画面から分けて、乗っ取りに気づいたときに、本人確認で待たせずに使えるようにする。
function SessionsPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="md">
      <Anchor component={Link} to="/settings" size="sm">
        {t('settings.back')}
      </Anchor>
      <Title order={2}>{t('settings.pages.sessions.title')}</Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Sessions />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Sessions() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // fetchKey を変えると、読み直す。ログアウトさせた後に、一覧を新しくするのに使う。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの端末で設定を変えたら、読み直す (#503)。
  const viewerChanges = useDataChanges(['VIEWER_SETTINGS']);
  const data = useLazyLoadQuery<sessionsQuery>(
    graphql`
      query sessionsQuery {
        sessions {
          id
          current
          createdAt
          lastSeenAt
          userAgent
          sourceIp
        }
        oauthGrants {
          id
          clientId
          clientName
          scopes
          createdAt
          lastUsedAt
        }
      }
    `,
    {},
    { fetchKey: `${fetchKey}:${viewerChanges}`, fetchPolicy: 'store-and-network' },
  );
  const [error, setError] = useState<string | null>(null);
  // revokedOthers は、ほかの端末をまとめてログアウトさせた数。終えたことを知らせるのに使う。
  const [revokedOthers, setRevokedOthers] = useState<number | null>(null);
  // confirming は、ほかの端末をまとめてログアウトさせる前に、確かめているかどうか。
  const [confirming, setConfirming] = useState(false);
  const others = data.sessions.filter((session) => !session.current).length;
  // running は、ログアウトさせるか、連携を取り消している途中かどうか。続けて押されても、重ねて送らない。
  const [running, setRunning] = useState(false);
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const fail = (caught: unknown) => setError(settingsErrorMessage(t, caught));
  const run = (operation: () => Promise<void>, onError: (error: unknown) => void) => {
    setError(null);
    setRevokedOthers(null);
    setRunning(true);
    operation()
      .catch(onError)
      .finally(() => setRunning(false));
  };

  const revoke = (id: string) =>
    run(async () => {
      await mutate<sessionsRevokeMutation>(
        environment,
        graphql`
          mutation sessionsRevokeMutation($id: ID!) {
            revokeSession(id: $id)
          }
        `,
        { id },
      );
      refresh();
    }, fail);
  // revokeOthers は、この端末のほかを、まとめてログアウトさせる。使わなくなった端末で、一覧が長くなったときに使う。
  const revokeOthers = () => {
    setConfirming(false);
    run(async () => {
      const response = await mutate<sessionsRevokeOthersMutation>(
        environment,
        graphql`
          mutation sessionsRevokeOthersMutation {
            revokeOtherSessions
          }
        `,
        {},
      );
      setRevokedOthers(response.revokeOtherSessions);
      refresh();
    }, fail);
  };

  return (
    <Stack gap="md">
      {error !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {revokedOthers !== null && (
        <Alert color="teal" role="status" withCloseButton onClose={() => setRevokedOthers(null)}>
          {t('settings.account.revokedOthers', { count: revokedOthers })}
        </Alert>
      )}
      {/* 操作の途中は、ほかのボタンを押せないようにする。 */}
      <Fieldset variant="unstyled" disabled={running} p={0} m={0} miw={0}>
        <Stack gap="md">
          <Card withBorder>
            <Stack gap="xs">
              <Group justify="space-between" wrap="wrap">
                <Title order={3} size="h5">
                  {t('settings.account.sessions')}
                </Title>
                {others > 0 && (
                  <Button
                    size="compact-sm"
                    variant="default"
                    color="red"
                    onClick={() => setConfirming(true)}
                  >
                    {t('settings.account.revokeOthers')}
                  </Button>
                )}
              </Group>
              {confirming && (
                <Alert color="red" role="alert">
                  <Stack gap="xs">
                    <Text size="sm">
                      {t('settings.account.revokeOthersConfirm', { count: others })}
                    </Text>
                    <Group gap="xs">
                      <Button size="compact-sm" color="red" onClick={revokeOthers}>
                        {t('settings.account.revokeOthers')}
                      </Button>
                      <Button
                        size="compact-sm"
                        variant="subtle"
                        onClick={() => setConfirming(false)}
                      >
                        {t('settings.password.cancel')}
                      </Button>
                    </Group>
                  </Stack>
                </Alert>
              )}
              {data.sessions.map((session) => (
                <Group
                  key={session.id}
                  justify="space-between"
                  wrap="nowrap"
                  data-session={session.id}
                >
                  <Stack gap={0} miw={0} flex={1}>
                    <Group gap="xs">
                      <Text size="sm" truncate title={session.userAgent}>
                        {deviceName(session.userAgent) ??
                          (session.userAgent || t('settings.account.unknownDevice'))}
                      </Text>
                      {session.current && (
                        <Badge size="xs" variant="light">
                          {t('settings.account.thisSession')}
                        </Badge>
                      )}
                    </Group>
                    <Text size="xs" c="dimmed">
                      {t('settings.account.sessionTimes', {
                        lastSeen: dateTimeFormat.format(Date.parse(session.lastSeenAt)),
                        source: session.sourceIp ?? '-',
                      })}
                    </Text>
                  </Stack>
                  {!session.current && (
                    <Button
                      size="compact-sm"
                      variant="default"
                      flex="none"
                      onClick={() => revoke(session.id)}
                    >
                      {t('settings.account.revoke')}
                    </Button>
                  )}
                </Group>
              ))}
            </Stack>
          </Card>
          <OAuthGrants
            grants={data.oauthGrants}
            onChanged={refresh}
            run={run}
            fail={fail}
            environment={environment}
          />
        </Stack>
      </Fieldset>
    </Stack>
  );
}
