import { ThemeIcon, Tooltip } from '@mantine/core';
import { CloudOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { useSubscriptionsReconnecting } from './connection';

// ConnectionStatus は、サーバーからの変更の知らせ (subscription) が切れ、つなぎ直している間だけ、ヘッダーに出す印 (#503)。
// 切れている間も画面は使えるので、操作を止めず、アイコンとツールチップで知らせるだけにする。
export function ConnectionStatus() {
  const { t } = useTranslation();
  const reconnecting = useSubscriptionsReconnecting();
  if (!reconnecting) {
    return null;
  }

  return (
    <Tooltip label={t('connection.reconnecting')}>
      <ThemeIcon
        variant="subtle"
        color="orange"
        size="lg"
        role="status"
        aria-label={t('connection.reconnecting')}
      >
        <CloudOff size={20} aria-hidden />
      </ThemeIcon>
    </Tooltip>
  );
}
