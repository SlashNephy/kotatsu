import { Anchor, AppShell, Group, Title } from '@mantine/core';
import { createRootRoute, Link, Outlet } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';

import { ColorSchemeToggle } from '../futon';
import { NotificationBell } from '../notifications/NotificationBell';

export const Route = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFound,
});

function RootLayout() {
  const { t } = useTranslation();

  return (
    <AppShell header={{ height: 64 }} padding="md">
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between">
          <Group gap="lg">
            <Title order={1} size="h3">
              {t('app.name')}
            </Title>
            <Group gap="md" component="nav">
              <Anchor component={Link} to="/" size="sm">
                {t('nav.home')}
              </Anchor>
              <Anchor component={Link} to="/guide" size="sm">
                {t('nav.guide')}
              </Anchor>
              <Anchor component={Link} to="/live" size="sm">
                {t('nav.live')}
              </Anchor>
              <Anchor component={Link} to="/search" size="sm">
                {t('nav.search')}
              </Anchor>
              <Anchor component={Link} to="/reservations" size="sm">
                {t('nav.reservations')}
              </Anchor>
              <Anchor component={Link} to="/rules" size="sm">
                {t('nav.rules')}
              </Anchor>
              <Anchor component={Link} to="/recordings" size="sm">
                {t('nav.recordings')}
              </Anchor>
              <Anchor component={Link} to="/settings" size="sm">
                {t('nav.settings')}
              </Anchor>
            </Group>
          </Group>
          <Group gap="md">
            <NotificationBell />
            <ColorSchemeToggle />
          </Group>
        </Group>
      </AppShell.Header>
      <AppShell.Main>
        <Outlet />
      </AppShell.Main>
    </AppShell>
  );
}

function NotFound() {
  const { t } = useTranslation();

  return (
    <>
      <Title order={2}>{t('notFound.title')}</Title>
      <Link to="/">{t('notFound.back')}</Link>
    </>
  );
}
