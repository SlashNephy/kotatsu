import { Anchor, AppShell, Box, Burger, Group, Stack, Title } from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { createRootRoute, Link, Outlet } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';

import { ColorSchemeToggle } from '../futon';
import { NotificationBell } from '../notifications/NotificationBell';

export const Route = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFound,
});

// navItems は、ヘッダー (広い画面) とナビゲーションの引き出し (狭い画面) に出すリンク。
const navItems = [
  { to: '/', key: 'nav.home' },
  { to: '/guide', key: 'nav.guide' },
  { to: '/live', key: 'nav.live' },
  { to: '/search', key: 'nav.search' },
  { to: '/reservations', key: 'nav.reservations' },
  { to: '/rules', key: 'nav.rules' },
  { to: '/recordings', key: 'nav.recordings' },
  { to: '/settings', key: 'nav.settings' },
] as const;

// RootLayout は、すべての画面の枠。広い画面では、ヘッダーにリンクを並べる。
// 狭い画面では、リンクが収まらず重なるので、ヘッダーのボタンで開く引き出しに移す。
function RootLayout() {
  const { t } = useTranslation();
  const [opened, { toggle, close }] = useDisclosure(false);

  return (
    <AppShell
      header={{ height: 64 }}
      navbar={{ width: 240, breakpoint: 'sm', collapsed: { mobile: !opened, desktop: true } }}
      padding="md"
    >
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between" wrap="nowrap">
          <Group gap="lg" wrap="nowrap">
            <Burger
              opened={opened}
              onClick={toggle}
              hiddenFrom="sm"
              size="sm"
              aria-label={t('nav.menu')}
            />
            <Title order={1} size="h3">
              {t('app.name')}
            </Title>
            <Group gap="md" component="nav" visibleFrom="sm">
              {navItems.map((item) => (
                <Anchor key={item.to} component={Link} to={item.to} size="sm">
                  {t(item.key)}
                </Anchor>
              ))}
            </Group>
          </Group>
          <Group gap="md" wrap="nowrap">
            <NotificationBell />
            <Box visibleFrom="sm">
              <ColorSchemeToggle />
            </Box>
          </Group>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar p="md" component="nav" aria-label={t('nav.menu')}>
        <Stack gap="sm">
          {navItems.map((item) => (
            <Anchor key={item.to} component={Link} to={item.to} onClick={close}>
              {t(item.key)}
            </Anchor>
          ))}
          <ColorSchemeToggle />
        </Stack>
      </AppShell.Navbar>
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
