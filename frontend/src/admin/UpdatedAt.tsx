import { Text } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { zonedFormat } from '../futon';

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// UpdatedAt は、全体の設定 (systemSettings) を変える画面の、最終更新の時刻。
// 最終更新は、全体の設定のどの項目を変えても進むので、ほかの画面で変えた時刻のこともある。
// 設定は変えたらすぐ保存するので (#1740)、保存のボタンは置かない。
export function UpdatedAt({ updatedAt }: { updatedAt: string | null | undefined }) {
  const { t } = useTranslation();

  return (
    <Text size="xs" c="dimmed">
      {updatedAt == null
        ? t('admin.settings.neverUpdated')
        : t('admin.settings.updatedAt', { at: dateTimeFormat.format(Date.parse(updatedAt)) })}
    </Text>
  );
}
