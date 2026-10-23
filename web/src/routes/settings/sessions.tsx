import { Alert, Anchor, Badge, Button, Card, Group, Stack, Text, Title } from '@mantine/core';
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
import type { sessionsQuery } from './__generated__/sessionsQuery.graphql';
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

// SessionsPage は、ログイン中の端末の一覧と、ほかの端末をログアウトさせる画面 (設計書 9.5、10.3、#980)。
// ログアウトさせるのは、乗っ取られた後の被害を抑える操作なので、再認証を求めない。セキュリティの画面から分けて、
// 乗っ取りに気づいたときに、本人確認で待たせずに使えるようにする。
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
      }
    `,
    {},
    { fetchKey: `${fetchKey}:${viewerChanges}`, fetchPolicy: 'store-and-network' },
  );
  const [error, setError] = useState<string | null>(null);
  // revoking は、ログアウトさせている途中かどうか。続けて押されても、重ねて送らない。
  const [revoking, setRevoking] = useState(false);

  const revoke = (id: string) => {
    setError(null);
    setRevoking(true);
    mutate<sessionsRevokeMutation>(
      environment,
      graphql`
        mutation sessionsRevokeMutation($id: ID!) {
          revokeSession(id: $id)
        }
      `,
      { id },
    )
      .then(() => startTransition(() => setFetchKey((key) => key + 1)))
      .catch((caught: unknown) => setError(settingsErrorMessage(t, caught)))
      .finally(() => setRevoking(false));
  };

  return (
    <Stack gap="md">
      {error !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      <Card withBorder>
        <Stack gap="xs">
          {data.sessions.map((session) => (
            <Group key={session.id} justify="space-between" wrap="nowrap" data-session={session.id}>
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
                  disabled={revoking}
                  onClick={() => revoke(session.id)}
                >
                  {t('settings.account.revoke')}
                </Button>
              )}
            </Group>
          ))}
        </Stack>
      </Card>
    </Stack>
  );
}
