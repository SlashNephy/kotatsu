import { Alert, Card, Stack, Text, Title } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { Suspense, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useSubscription } from 'react-relay';

import { ErrorBoundary } from '../futon';
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
      <ErrorBoundary fallback={() => <Alert color="amber">{t('viewer.signedOut')}</Alert>}>
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Viewer />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
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
      <Text>{t('viewer.greeting', { name: data.viewer.displayName })}</Text>
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
