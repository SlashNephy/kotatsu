import { Anchor, AppShell, Burger, Group, Stack, Title } from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { createRootRoute, Link, Outlet, useMatches, useRouterState } from '@tanstack/react-router';
import { Suspense, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { UserMenu } from '../auth/UserMenu';
import { DataChangesProvider, DataChangesSubscriber } from '../changes/DataChanges';
import { ColorSchemeToggle, ErrorBoundary, useThemeColor } from '../futon';
import { NotificationBell } from '../notifications/NotificationBell';
import { ConnectionStatus } from '../relay/ConnectionStatus';
import type { RootViewerQuery } from './__generated__/RootViewerQuery.graphql';
import classes from './__root.module.css';

declare module '@tanstack/react-router' {
  interface StaticDataRouteOption {
    // fluid は、本文の幅の上限を外す画面に付ける。
    fluid?: boolean;
  }
}

export const Route = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFound,
});

// navItems は、ヘッダー (広い画面) とナビゲーションの引き出し (狭い画面) に出すリンク。
// ホームはアプリ名から、設定はユーザーのメニューから開くので、ここには並べない。
const navItems = [
  { to: '/live', key: 'nav.live' },
  { to: '/guide', key: 'nav.guide' },
  { to: '/search', key: 'nav.search' },
  { to: '/reservations', key: 'nav.reservations' },
  { to: '/rules', key: 'nav.rules' },
  { to: '/recordings', key: 'nav.recordings' },
] as const;

// RootLayout は、すべての画面の枠。広い画面では、ヘッダーにリンクを並べる。
// 狭い画面では、リンクが収まらず重なるので、ヘッダーのボタンで開く引き出しに移す。
// ログインしていない間 (ログインと初回セットアップの画面) は、使えないリンクと通知を出さず、配色の切り替えだけを出す。
function RootLayout() {
  const { t } = useTranslation();
  const [opened, { toggle, close }] = useDisclosure(false);
  const fluid = useMatches({
    select: (matches) => matches.some((match) => match.staticData.fluid),
  });
  useThemeColor();
  // アプリのアイコンと名前は、押すとホームを開く。アイコンは飾りなので、読み上げない。
  const brand = (
    <Title order={1} size="h3">
      <Anchor
        component={Link}
        to="/"
        inherit
        c="inherit"
        underline="never"
        onClick={close}
        className={classes.brand}
      >
        <img
          src={`${import.meta.env.BASE_URL}icons/icon.svg`}
          alt=""
          width={36}
          height={36}
          className={classes.brandIcon}
        />
        {t('app.name')}
      </Anchor>
    </Title>
  );

  // データの変更は、ヘッダーで購読し、画面全体に配る (#503)。
  return (
    <DataChangesProvider>
      <AppShell
        // ステータスバーの下まで描くときは、その分だけヘッダーを高くする (#425)。本文の上の余白も、この高さに揃う。
        header={{ height: 'calc(64px + env(safe-area-inset-top))' }}
        navbar={{ width: 240, breakpoint: 'sm', collapsed: { mobile: !opened, desktop: true } }}
        padding="md"
      >
        <AppShell.Header className={classes.shellHeader}>
          <SignedIn
            fallback={
              <Group
                h="100%"
                px="md"
                justify="space-between"
                wrap="nowrap"
                className={classes.header}
                data-fluid={fluid || undefined}
              >
                {brand}
                <ColorSchemeToggle />
              </Group>
            }
          >
            <Group
              h="100%"
              px="md"
              justify="space-between"
              wrap="nowrap"
              className={classes.header}
              data-fluid={fluid || undefined}
            >
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
                      className={classes.navLink}
                      size="sm"
                    >
                      {t(item.key)}
                    </Anchor>
                  ))}
                </Group>
              </Group>
              <Group gap="md" wrap="nowrap">
                <DataChangesSubscriber />
                <ConnectionStatus />
                <NotificationBell />
                <UserMenu />
              </Group>
            </Group>
          </SignedIn>
        </AppShell.Header>
        <AppShell.Navbar className={classes.shellNavbar} component="nav" aria-label={t('nav.menu')}>
          <SignedIn fallback={null}>
            <Stack gap="sm">
              {navItems.map((item) => (
                <Anchor
                  key={item.to}
                  component={Link}
                  to={item.to}
                  className={classes.navLink}
                  onClick={close}
                >
                  {t(item.key)}
                </Anchor>
              ))}
            </Stack>
          </SignedIn>
        </AppShell.Navbar>
        <AppShell.Main className={classes.shellMain}>
          <div className={classes.content} data-fluid={fluid || undefined}>
            <Outlet />
          </div>
        </AppShell.Main>
      </AppShell>
    </DataChangesProvider>
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
