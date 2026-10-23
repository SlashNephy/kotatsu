import { Card, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { ErrorBoundary } from '../../futon';
import type { settingsViewerQuery } from './__generated__/settingsViewerQuery.graphql';

export const Route = createFileRoute('/settings/')({
  component: SettingsPage,
});

type SettingsLink = { to: string; title: string; body: string };

// SettingsPage は、設定の入り口 (設計書 10.3)。管理者には、管理者向けの画面へのリンクも出す。
function SettingsPage() {
  const { t } = useTranslation();
  const links: SettingsLink[] = [
    {
      to: '/settings/player',
      title: t('settings.pages.player.title'),
      body: t('settings.pages.player.summary'),
    },
    {
      to: '/settings/guide',
      title: t('settings.pages.guide.title'),
      body: t('settings.pages.guide.summary'),
    },
    {
      to: '/settings/notifications',
      title: t('notifications.settingsTitle'),
      body: t('settings.notificationsSummary'),
    },
    {
      to: '/settings/security',
      title: t('settings.pages.security.title'),
      body: t('settings.pages.security.summary'),
    },
    {
      to: '/settings/sessions',
      title: t('settings.pages.sessions.title'),
      body: t('settings.pages.sessions.summary'),
    },
  ];

  return (
    <Stack gap="sm">
      <Title order={2}>{t('settings.title')}</Title>
      <Links links={links} />
      {/* 管理者かどうかを確かめられなければ、管理者向けのリンクは出さない。画面を使えるかは、サーバーが確かめる。 */}
      <ErrorBoundary fallback={() => null}>
        <Suspense fallback={null}>
          <AdminLinks />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function AdminLinks() {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<settingsViewerQuery>(
    graphql`
      query settingsViewerQuery {
        viewer {
          role
        }
      }
    `,
    {},
  );
  if (data.viewer.role !== 'ADMIN') {
    return null;
  }

  return (
    <>
      <Title order={3} size="h5" mt="sm">
        {t('settings.adminSection')}
      </Title>
      <Links
        links={[
          {
            to: '/admin/users',
            title: t('admin.users.title'),
            body: t('admin.users.summary'),
          },
          {
            to: '/admin/webhooks',
            title: t('admin.webhooks.title'),
            body: t('admin.webhooks.summary'),
          },
          {
            to: '/admin/settings',
            title: t('admin.settings.title'),
            body: t('admin.settings.summary'),
          },
          {
            to: '/admin/channels',
            title: t('admin.channels.title'),
            body: t('admin.channels.summary'),
          },
          {
            to: '/admin/system',
            title: t('admin.system.title'),
            body: t('admin.system.summary'),
          },
          {
            to: '/admin/audit',
            title: t('admin.audit.title'),
            body: t('admin.audit.summary'),
          },
        ]}
      />
    </>
  );
}

function Links({ links }: { links: SettingsLink[] }) {
  return links.map((link) => (
    <Card
      key={link.to}
      withBorder
      padding="sm"
      renderRoot={(props) => <Link {...props} to={link.to} />}
    >
      <Text fw={600}>{link.title}</Text>
      <Text size="sm" c="dimmed">
        {link.body}
      </Text>
    </Card>
  ));
}
