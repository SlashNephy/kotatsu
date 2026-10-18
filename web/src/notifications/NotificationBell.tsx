import { ActionIcon, Indicator, Tooltip } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { Bell } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { useUnreadNotificationCount } from '../changes/DataChanges';

// NotificationBell は、ヘッダーに出す、通知の一覧への入り口と、未読の通知の数 (設計書 6.2)。
// 場所を取らないようにベルのアイコンにし、名前はツールチップと読み上げで補う。
// 数は、DataChangesSubscriber がまとめて購読する知らせで届くので、ほかの端末で既読にしても減る。
// 数が分からない間 (つなぐ前や切れたとき) も、一覧へは移れるようにする。
export function NotificationBell() {
  const { t } = useTranslation();
  const unread = useUnreadNotificationCount();

  return (
    <Tooltip label={t('nav.notifications')}>
      <Indicator
        label={unread ?? 0}
        size={16}
        offset={4}
        disabled={unread === null || unread === 0}
        color="mikan"
      >
        <ActionIcon
          component={Link}
          to="/notifications"
          variant="subtle"
          color="gray"
          size="lg"
          aria-label={
            unread === null
              ? t('nav.notifications')
              : t('notifications.bellLabel', { count: unread })
          }
        >
          <Bell size={20} aria-hidden />
        </ActionIcon>
      </Indicator>
    </Tooltip>
  );
}
