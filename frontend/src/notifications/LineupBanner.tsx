import { Alert, Anchor } from '@mantine/core';
import { Link, useRouterState } from '@tanstack/react-router';
import { Suspense, useState, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';

import { ErrorBoundary } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { useUnreadNotificationCount } from '../changes/DataChanges';
import type { LineupBannerQuery } from './__generated__/LineupBannerQuery.graphql';
import type { LineupBannerViewerQuery } from './__generated__/LineupBannerViewerQuery.graphql';
import {
  isLineupAlertDismissed,
  lineupDismissalsVersion,
  recentLineupAlerts,
  subscribeLineupDismissals,
} from './lineupAlerts';

// systemPath は、チャンネルの変更を確かめるシステムの状態の画面。開くと、お知らせを既読にする。
const systemPath = '/admin/system';

// LineupBanner は、衛星の局の顔ぶれがチューナーサーバーと食い違ったこと (チャンネルの変更、#1519) を、画面の上の 1 行で知らせる。
// 知らせは管理者にだけ届くので、管理者のときだけ読む。システムの状態の画面を開くまで、出しておく長さの間だけ出す。
export function LineupBanner() {
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  // 自分のロールは、アプリの根元で読んだものを使い、問い合わせない。
  const { viewer } = useLazyLoadQuery<LineupBannerViewerQuery>(
    graphql`
      query LineupBannerViewerQuery {
        viewer {
          role
        }
      }
    `,
    {},
    { fetchPolicy: 'store-or-network' },
  );
  if (viewer.role !== 'ADMIN' || pathname === systemPath) {
    // システムの状態の画面には、同じ知らせを出し、開いたら既読にする。
    return null;
  }

  // 読めなくても、本文は出す。画面を移ったら、読み直す。
  return (
    <ErrorBoundary resetKey={pathname} fallback={() => null}>
      <Suspense fallback={null}>
        <LineupBannerContent />
      </Suspense>
    </ErrorBoundary>
  );
}

function LineupBannerContent() {
  const { t } = useTranslation();
  // 未読の通知の数が変わったら (知らせが届いたか、既読にしたら)、読み直す。画面を移るだけでは読み直さない。
  const unread = useUnreadNotificationCount();
  const data = useLazyLoadQuery<LineupBannerQuery>(
    graphql`
      query LineupBannerQuery {
        notifications(first: 5, unreadOnly: true, kinds: [LINEUP_CHANGED]) {
          edges {
            node {
              id
              message
              occurredAt
              readAt
            }
          }
        }
      }
    `,
    {},
    { fetchKey: unread ?? -1, fetchPolicy: 'store-and-network' },
  );
  const [now] = useState(Date.now);
  // システムの状態の画面で既読にしたものは、読み直す前でも出さない。
  useSyncExternalStore(subscribeLineupDismissals, lineupDismissalsVersion);
  const [latest] = recentLineupAlerts(
    data.notifications.edges
      .map(({ node }) => node)
      .filter((node) => node.readAt == null && !isLineupAlertDismissed(node.id)),
    now,
  );
  if (latest === undefined) {
    return null;
  }

  return (
    <Alert color="amber" mb="sm" role="status">
      {latest.message}{' '}
      <Anchor component={Link} to={systemPath} size="sm">
        {t('notifications.lineup.open')}
      </Anchor>
    </Alert>
  );
}
