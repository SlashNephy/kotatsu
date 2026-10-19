import { Alert, Stack, Text } from '@mantine/core';
import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { startTransition, Suspense, useCallback, useEffect, useState } from 'react';

import { Dashboard, type DashboardRange } from '../home/Dashboard';
import { useTranslation } from 'react-i18next';

import { authApi } from '../auth/api';
import { ErrorBoundary, PageLoader } from '../futon';
import { isUnauthenticated } from '../relay/environment';

export const Route = createFileRoute('/')({
  component: Home,
});

// upcomingDays は、ホームに出す、これからの予約と予約の競合の期間 (日)。
const upcomingDays = 7;

// homeRange は、now を基準にした、ホームで読む時間帯。
function homeRange(now: number): DashboardRange {
  return {
    now,
    from: new Date(now).toISOString(),
    // 放送中の番組は、今に重なる番組を引けば足りる。
    nowEnd: new Date(now + 1000).toISOString(),
    to: new Date(now + upcomingDays * 24 * 60 * 60 * 1000).toISOString(),
  };
}

function Home() {
  const { t } = useTranslation();
  // 時刻は、待たない親で決める。待つ子で決めると、待つたびに決め直して、読み込みが終わらなくなる。
  const [range, setRange] = useState(() => homeRange(Date.now()));
  // 放送中の番組が終わったり、予約が始まったりしたら、時刻を決め直して読み直す (#866)。
  // 読み直す間も前の中身を出したままにするため、transition の中で変える。
  const refresh = useCallback(() => startTransition(() => setRange(homeRange(Date.now()))), []);

  return (
    <Stack>
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
        <Suspense fallback={<PageLoader />}>
          <Dashboard range={range} onExpire={refresh} />
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
