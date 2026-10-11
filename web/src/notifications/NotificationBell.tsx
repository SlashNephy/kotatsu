import { Anchor, Indicator } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useSubscription } from 'react-relay';

import type { NotificationBellSubscription } from './__generated__/NotificationBellSubscription.graphql';

// NotificationBell は、ヘッダーに出す、未読の通知の数 (設計書 6.2)。数は subscription で届くので、ほかの端末で既読にしても減る。
// ログインしていなければ、購読できないので出さない。
export function NotificationBell() {
  const { t } = useTranslation();
  const [unread, setUnread] = useState<number | null>(null);
  const config = useMemo(
    () => ({
      subscription: graphql`
        subscription NotificationBellSubscription {
          unreadNotificationCount
        }
      `,
      variables: {},
      onNext: (response: NotificationBellSubscription['response'] | null | undefined) => {
        if (response) {
          setUnread(response.unreadNotificationCount);
        }
      },
      onError: () => setUnread(null),
    }),
    [],
  );
  useSubscription<NotificationBellSubscription>(config);

  if (unread === null) {
    return null;
  }

  return (
    <Indicator label={unread} size={16} disabled={unread === 0} color="mikan">
      <Anchor
        component={Link}
        to="/notifications"
        size="sm"
        aria-label={t('notifications.unread', { count: unread })}
      >
        {t('nav.notifications')}
      </Anchor>
    </Indicator>
  );
}
