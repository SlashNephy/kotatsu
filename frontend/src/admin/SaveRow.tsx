import { Button, Group, Text } from '@mantine/core';
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

// SaveRow は、全体の設定 (systemSettings) を変える画面の、最終更新の時刻と保存のボタン。
// 最終更新は、全体の設定のどの項目を変えても進むので、ほかの画面で変えた時刻のこともある。
export function SaveRow({
  updatedAt,
  disabled,
}: {
  updatedAt: string | null | undefined;
  disabled: boolean;
}) {
  const { t } = useTranslation();

  return (
    <Group justify="space-between">
      <Text size="xs" c="dimmed">
        {updatedAt == null
          ? t('admin.settings.neverUpdated')
          : t('admin.settings.updatedAt', { at: dateTimeFormat.format(Date.parse(updatedAt)) })}
      </Text>
      <Button type="submit" size="compact-sm" disabled={disabled}>
        {t('admin.save')}
      </Button>
    </Group>
  );
}
