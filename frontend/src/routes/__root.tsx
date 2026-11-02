import { ActionIcon, Anchor, AppShell, Burger, Group, Stack, Title, Tooltip } from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { createRootRoute, Link, Outlet, useMatches, useRouterState } from '@tanstack/react-router';
import { HardDrive } from 'lucide-react';
import { Suspense, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { UserMenu } from '../auth/UserMenu';
import { DataChangesProvider, DataChangesSubscriber } from '../changes/DataChanges';
import { ColorSchemeToggle, ErrorBoundary, useThemeColor } from '../futon';
import { NotificationBell } from '../notifications/NotificationBell';
import { useOfflineRecordings } from '../offline/store';
import { ConnectionStatus } from '../relay/ConnectionStatus';
import type { RootNavQuery } from './__generated__/RootNavQuery.graphql';
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

// animeNavItem は、管理者がアニメの連携を有効にしているときだけ出す、視聴管理の画面へのリンク (#1231)。
const animeNavItem = { to: '/anime', key: 'nav.anime' } as const;

type NavItem = (typeof navItems)[number] | typeof animeNavItem;

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
                <Group gap="md" wrap="nowrap">
                  <OfflineShortcut />
                  <ColorSchemeToggle />
                </Group>
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
                  <NavLinks size="sm" />
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
              <NavLinks onClick={close} />
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

const rootViewerQuery = graphql`
  query RootViewerQuery {
    viewer {
      username
    }
  }
`;

// ViewerGate は、自分のユーザーを読めたときだけ children を出す。読めなければ、読み込みが例外を投げる。
// 失敗した読み込みは Relay が覚えているので、fetchKey を変えて読み直させる。読めた後は、ストアの値を使う。
function ViewerGate({ children, fetchKey }: { children: ReactNode; fetchKey: string }) {
  useLazyLoadQuery<RootViewerQuery>(
    rootViewerQuery,
    {},
    { fetchKey, fetchPolicy: 'store-or-network' },
  );

  return children;
}

// NavLinks は、ヘッダーと引き出しに並べるリンク。視聴管理の画面へのリンクは、管理者がアニメの連携を有効にしているときだけ出す (#1231)。
// 連携の設定を読めない間と、読めなかったときは、ほかのリンクだけを出す (ヘッダーまで出なくならないようにする)。
function NavLinks({ size, onClick }: { size?: 'sm'; onClick?: () => void }) {
  const fallback = <NavAnchors items={navItems} size={size} onClick={onClick} />;

  return (
    <ErrorBoundary fallback={() => fallback}>
      <Suspense fallback={fallback}>
        <AnimeAwareNavLinks size={size} onClick={onClick} />
      </Suspense>
    </ErrorBoundary>
  );
}

function AnimeAwareNavLinks({ size, onClick }: { size?: 'sm'; onClick?: () => void }) {
  const data = useLazyLoadQuery<RootNavQuery>(
    graphql`
      query RootNavQuery {
        animeRecordSettings {
          enabled
        }
      }
    `,
    {},
    { fetchPolicy: 'store-or-network' },
  );
  const items = data.animeRecordSettings.enabled ? [...navItems, animeNavItem] : navItems;

  return <NavAnchors items={items} size={size} onClick={onClick} />;
}

function NavAnchors({
  items,
  size,
  onClick,
}: {
  items: readonly NavItem[];
  size?: 'sm';
  onClick?: () => void;
}) {
  const { t } = useTranslation();

  return items.map((item) => (
    <Anchor
      key={item.to}
      component={Link}
      to={item.to}
      className={classes.navLink}
      size={size}
      onClick={onClick}
    >
      {t(item.key)}
    </Anchor>
  ));
}

// OfflineShortcut は、ログインしていないとき (オフラインで自分のユーザーを読めないときを含む) のヘッダーに出す、
// 端末に保存した録画 (#480) へのリンク。オフラインでは、ほかのリンクが出ないので、ここから開けるようにする。
// 保存した録画がなければ出さない。
function OfflineShortcut() {
  const { t } = useTranslation();
  const { saved } = useOfflineRecordings();
  if (saved === null || saved.length === 0) {
    return null;
  }

  return (
    <Tooltip label={t('nav.offline')}>
      <ActionIcon
        component={Link}
        to="/offline"
        variant="subtle"
        size="lg"
        aria-label={t('nav.offline')}
      >
        <HardDrive size={20} aria-hidden />
      </ActionIcon>
    </Tooltip>
  );
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
