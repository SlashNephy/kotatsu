import { Alert, Button, Card, Group, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { Suspense, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useSubscription } from 'react-relay';

import { authApi } from '../auth/api';
import { ErrorBoundary } from '../futon';
import { isUnauthenticated } from '../relay/environment';
import type { routesHeartbeatSubscription } from './__generated__/routesHeartbeatSubscription.graphql';
import type { routesViewerQuery } from './__generated__/routesViewerQuery.graphql';

export const Route = createFileRoute('/')({
  component: Home,
});

function Home() {
  const { t } = useTranslation();

  return (
    <Stack>
      <Title order={2}>{t('app.name')}</Title>
      <Text>{t('app.tagline')}</Text>
      <ErrorBoundary
        fallback={(error) =>
          // オフラインでも、Service Worker が外枠を返すので、ここまでは表示される。
          !navigator.onLine ? (
            <Alert color="amber">{t('viewer.offline')}</Alert>
          ) : isUnauthenticated(error) ? (
            <SignedOut />
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Viewer />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// SignedOut は、ログインしていないときに、初回セットアップかログインの画面に移る (設計書 9.2)。
function SignedOut() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  useEffect(() => {
    authApi.setupStatus().then(
      // 戻るボタンで、ここに戻ってまた移る、を繰り返さないよう、履歴を置き換える。
      (status) => navigate({ to: status.required ? '/setup' : '/login', replace: true }),
      () => navigate({ to: '/login', replace: true }),
    );
  }, [navigate]);

  return <Text c="dimmed">{t('viewer.signedOut')}</Text>;
}

function Viewer() {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<routesViewerQuery>(
    graphql`
      query routesViewerQuery {
        viewer {
          displayName
          role
        }
      }
    `,
    {},
  );

  return (
    <Card withBorder>
      <Group justify="space-between">
        <Text>{t('viewer.greeting', { name: data.viewer.displayName })}</Text>
        <Button
          variant="subtle"
          size="xs"
          onClick={() => {
            void authApi.logout().then(() => window.location.assign('/login'));
          }}
        >
          {t('auth.logout')}
        </Button>
      </Group>
      <Heartbeat />
    </Card>
  );
}

// Heartbeat は、tenban から SSE で届く時刻を出す。subscription がつながっていることを確かめる。
function Heartbeat() {
  const { t } = useTranslation();
  const [now, setNow] = useState<string | null>(null);
  const config = useMemo(
    () => ({
      subscription: graphql`
        subscription routesHeartbeatSubscription {
          heartbeat
        }
      `,
      variables: {},
      onNext: (response: routesHeartbeatSubscription['response'] | null | undefined) => {
        if (response) {
          setNow(response.heartbeat);
        }
      },
    }),
    [],
  );
  useSubscription<routesHeartbeatSubscription>(config);

  return (
    <Text size="sm" c="dimmed">
      {now === null
        ? t('viewer.connecting')
        : t('viewer.heartbeat', { time: new Date(now).toLocaleString('ja-JP') })}
    </Text>
  );
}
