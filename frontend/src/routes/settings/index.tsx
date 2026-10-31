import { Card, Group, Stack, Text, ThemeIcon, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import {
  Activity,
  Bell,
  CalendarDays,
  CircleDot,
  HardDrive,
  Captions,
  Hourglass,
  MonitorPlay,
  MonitorSmartphone,
  Plug,
  Rewind,
  ScrollText,
  ShieldCheck,
  Tv,
  Users,
  type LucideIcon,
} from 'lucide-react';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { ErrorBoundary } from '../../futon';
import type { settingsViewerQuery } from './__generated__/settingsViewerQuery.graphql';

export const Route = createFileRoute('/settings/')({
  component: SettingsPage,
});

type SettingsLink = { to: string; title: string; body: string; icon: LucideIcon };

// SettingsPage は、設定の入り口 (設計書 10.3)。管理者には、管理者向けの画面へのリンクも出す。
function SettingsPage() {
  const { t } = useTranslation();
  const links: SettingsLink[] = [
    {
      to: '/settings/player',
      icon: MonitorPlay,
      title: t('settings.pages.player.title'),
      body: t('settings.pages.player.summary'),
    },
    {
      to: '/settings/guide',
      icon: CalendarDays,
      title: t('settings.pages.guide.title'),
      body: t('settings.pages.guide.summary'),
    },
    {
      to: '/settings/notifications',
      icon: Bell,
      title: t('notifications.settingsTitle'),
      body: t('settings.notificationsSummary'),
    },
    {
      to: '/settings/security',
      icon: ShieldCheck,
      title: t('settings.pages.security.title'),
      body: t('settings.pages.security.summary'),
    },
    {
      to: '/settings/sessions',
      icon: MonitorSmartphone,
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

  // 管理者向けの画面は、録画にかかわるもの、人と外部のサービスにかかわるもの、状態を確かめるものに分ける (設計書 10.3)。
  const groups: { title: string; links: SettingsLink[] }[] = [
    {
      title: t('settings.adminGroups.recording'),
      links: [
        {
          to: '/admin/recording',
          title: t('admin.recording.title'),
          body: t('admin.recording.summary'),
          icon: CircleDot,
        },
        {
          to: '/admin/services',
          title: t('admin.services.title'),
          body: t('admin.services.summary'),
          icon: Tv,
        },
        {
          to: '/admin/timeshift',
          title: t('admin.timeshift.title'),
          body: t('admin.timeshift.summary'),
          icon: Rewind,
        },
        {
          to: '/admin/storage',
          title: t('admin.storage.title'),
          body: t('admin.storage.summary'),
          icon: HardDrive,
        },
        {
          to: '/admin/captions',
          title: t('admin.captions.title'),
          body: t('admin.captions.summary'),
          icon: Captions,
        },
        {
          to: '/admin/retention',
          title: t('admin.retention.title'),
          body: t('admin.retention.summary'),
          icon: Hourglass,
        },
      ],
    },
    {
      title: t('settings.adminGroups.people'),
      links: [
        {
          to: '/admin/users',
          title: t('admin.users.title'),
          body: t('admin.users.summary'),
          icon: Users,
        },
        {
          to: '/admin/integrations',
          title: t('admin.integrations.title'),
          body: t('admin.integrations.summary'),
          icon: Plug,
        },
      ],
    },
    {
      title: t('settings.adminGroups.status'),
      links: [
        {
          to: '/admin/system',
          title: t('admin.system.title'),
          body: t('admin.system.summary'),
          icon: Activity,
        },
        {
          to: '/admin/audit',
          title: t('admin.audit.title'),
          body: t('admin.audit.summary'),
          icon: ScrollText,
        },
      ],
    },
  ];

  return (
    <>
      <Title order={3} size="h5" mt="sm">
        {t('settings.adminSection')}
      </Title>
      {groups.map((group) => (
        <Stack key={group.title} gap="sm">
          <Title order={4} size="h6" c="dimmed" mt="xs">
            {group.title}
          </Title>
          <Links links={group.links} />
        </Stack>
      ))}
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
      <Group gap="sm" wrap="nowrap" align="flex-start">
        <ThemeIcon variant="light" size="lg" radius="md" aria-hidden>
          <link.icon size={20} />
        </ThemeIcon>
        <div>
          <Text fw={600}>{link.title}</Text>
          <Text size="sm" c="dimmed">
            {link.body}
          </Text>
        </div>
      </Group>
    </Card>
  ));
}
