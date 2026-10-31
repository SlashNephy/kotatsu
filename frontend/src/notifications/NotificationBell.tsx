import {
  ActionIcon,
  Alert,
  Anchor,
  Group,
  Indicator,
  Loader,
  Popover,
  ScrollArea,
  Text,
  Tooltip,
} from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { Bell } from 'lucide-react';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useUnreadNotificationCount } from '../changes/DataChanges';
import { ErrorBoundary } from '../futon';
import { NotificationList } from './NotificationList';

// popoverSize は、ポップオーバーに出す通知の数。ほかの通知は、通知のページで見る。
const popoverSize = 10;

// NotificationBell は、ヘッダーに出す、通知の入り口と、未読の通知の数 (設計書 6.2)。
// 場所を取らないようにベルのアイコンにし、名前はツールチップと読み上げで補う。
// 押すと、新しい通知をポップオーバーで出し、通知のページへ移らずに読めるようにする (#1086)。
// 開いたときの既読の扱いは、通知のページと同じ。
// 数は、DataChangesSubscriber がまとめて購読する知らせで届くので、ほかの端末で既読にしても減る。
// 数が分からない間 (つなぐ前や切れたとき) も、通知は開けるようにする。
export function NotificationBell() {
  const { t } = useTranslation();
  const unread = useUnreadNotificationCount();
  const [opened, setOpened] = useState(false);
  const close = () => setOpened(false);

  return (
    <Popover
      opened={opened}
      onChange={setOpened}
      position="bottom-end"
      width="min(360px, calc(100vw - 32px))"
      shadow="md"
      withinPortal
      trapFocus
    >
      <Popover.Target>
        <Tooltip label={t('nav.notifications')} disabled={opened}>
          <Indicator
            label={unread ?? 0}
            size={16}
            offset={4}
            disabled={unread === null || unread === 0}
            color="mikan"
          >
            <ActionIcon
              variant="subtle"
              color="gray"
              size="lg"
              aria-label={
                unread === null
                  ? t('nav.notifications')
                  : t('notifications.bellLabel', { count: unread })
              }
              aria-expanded={opened}
              onClick={() => setOpened((value) => !value)}
            >
              <Bell size={20} aria-hidden />
            </ActionIcon>
          </Indicator>
        </Tooltip>
      </Popover.Target>
      <Popover.Dropdown p="xs">
        <Group justify="space-between" mb="xs">
          <Text fw={700} size="sm">
            {t('notifications.title')}
          </Text>
          <Anchor component={Link} to="/settings/notifications" size="xs" onClick={close}>
            {t('notifications.settings')}
          </Anchor>
        </Group>
        {/* 開いている間だけ読み込み、開くたびに既読にする。 */}
        {opened && (
          <ScrollArea.Autosize mah="min(60vh, 480px)" type="auto">
            <ErrorBoundary fallback={() => <Alert color="amber">{t('viewer.error')}</Alert>}>
              <Suspense fallback={<Loader size="sm" />}>
                <NotificationList first={popoverSize} onNavigate={close} />
              </Suspense>
            </ErrorBoundary>
          </ScrollArea.Autosize>
        )}
        <Group justify="center" mt="xs">
          <Anchor component={Link} to="/notifications" size="sm" onClick={close}>
            {t('notifications.viewAll')}
          </Anchor>
        </Group>
      </Popover.Dropdown>
    </Popover>
  );
}
