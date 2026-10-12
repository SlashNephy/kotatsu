import { Anchor, AppShell, Burger, Group, Stack, Title } from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { createRootRoute, Link, Outlet, useRouterState } from '@tanstack/react-router';
import { Suspense, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { UserMenu } from '../auth/UserMenu';
import { ColorSchemeToggle, ErrorBoundary } from '../futon';
import { NotificationBell } from '../notifications/NotificationBell';
import type { RootViewerQuery } from './__generated__/RootViewerQuery.graphql';
import classes from './__root.module.css';

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
// ログインしていない間 (ログインと初回セットアップの画面) は、使えないリンクと通知を出さず、配色の切り替えだけを出す。
function RootLayout() {
  const { t } = useTranslation();
  const [opened, { toggle, close }] = useDisclosure(false);
  const brand = (
    <Title order={1} size="h3">
      {t('app.name')}
    </Title>
  );

  return (
    <AppShell
      header={{ height: 64 }}
      navbar={{ width: 240, breakpoint: 'sm', collapsed: { mobile: !opened, desktop: true } }}
      padding="md"
    >
      <AppShell.Header>
        <SignedIn
          fallback={
            <Group h="100%" px="md" justify="space-between" wrap="nowrap">
              {brand}
              <ColorSchemeToggle />
            </Group>
          }
        >
          <Group h="100%" px="md" justify="space-between" wrap="nowrap">
            <Group gap="lg" wrap="nowrap">
              <Burger
                opened={opened}
                onClick={toggle}
                hiddenFrom="sm"
                size="sm"
                aria-label={t('nav.menu')}
              />
              {brand}
              <Group gap="md" component="nav" visibleFrom="sm">
                {navItems.map((item) => (
                  <Anchor
                    key={item.to}
                    component={Link}
                    to={item.to}
                    activeOptions={{ exact: item.to === '/' }}
                    className={classes.navLink}
                    size="sm"
                  >
                    {t(item.key)}
                  </Anchor>
                ))}
              </Group>
            </Group>
            <Group gap="md" wrap="nowrap">
              <NotificationBell />
              <UserMenu />
            </Group>
          </Group>
        </SignedIn>
      </AppShell.Header>
      <AppShell.Navbar p="md" component="nav" aria-label={t('nav.menu')}>
        <SignedIn fallback={null}>
          <Stack gap="sm">
            {navItems.map((item) => (
              <Anchor
                key={item.to}
                component={Link}
                to={item.to}
                activeOptions={{ exact: item.to === '/' }}
                className={classes.navLink}
                onClick={close}
              >
                {t(item.key)}
              </Anchor>
            ))}
          </Stack>
        </SignedIn>
      </AppShell.Navbar>
      <AppShell.Main>
        <Outlet />
      </AppShell.Main>
    </AppShell>
  );
}

// SignedIn は、ログインしていれば children を、していなければ (自分のユーザーを読めなければ) fallback を出す。
// 読み込みの間は、ヘッダーが空にならないよう、fallback を出す。
// 読み込みに一度失敗しても、画面を移ると読み直すよう、ErrorBoundary をパスごとに作り直す。
function SignedIn({ children, fallback }: { children: ReactNode; fallback: ReactNode }) {
  const pathname = useRouterState({ select: (state) => state.location.pathname });

  return (
    <ErrorBoundary key={pathname} fallback={() => fallback}>
      <Suspense fallback={fallback}>
        <ViewerGate fetchKey={pathname}>{children}</ViewerGate>
      </Suspense>
    </ErrorBoundary>
  );
}

// ViewerGate は、自分のユーザーを読めたときだけ children を出す。読めなければ、読み込みが例外を投げる。
// 失敗した読み込みは Relay が覚えているので、fetchKey を変えて読み直させる。読めた後は、ストアの値を使う。
function ViewerGate({ children, fetchKey }: { children: ReactNode; fetchKey: string }) {
  useLazyLoadQuery<RootViewerQuery>(
    graphql`
      query RootViewerQuery {
        viewer {
          username
        }
      }
    `,
    {},
    { fetchKey, fetchPolicy: 'store-or-network' },
  );

  return children;
}

function NotFound() {
  const { t } = useTranslation();

  return (
    <Stack gap="xs">
      <Title order={2}>{t('notFound.title')}</Title>
      <Anchor component={Link} to="/">
        {t('notFound.back')}
      </Anchor>
    </Stack>
  );
}
