import { Alert, Anchor, Group, Stack, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';

import { ErrorBoundary, PageLoader } from '../futon';
import { NotificationList } from '../notifications/NotificationList';
import { isUnauthenticated } from '../relay/environment';

export const Route = createFileRoute('/notifications')({
  component: NotificationsPage,
});

// NotificationsPage は、画面内の通知の一覧 (設計書 6.2)。開くと、その時点の通知をすべて既読にする。
function NotificationsPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="sm">
      <Group justify="space-between">
        <Title order={2}>{t('notifications.title')}</Title>
        <Anchor component={Link} to="/settings/notifications" size="sm">
          {t('notifications.settings')}
        </Anchor>
      </Group>
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
          <NotificationList first={pageSize} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// pageSize は、通知のページに出す通知の数。
const pageSize = 100;
