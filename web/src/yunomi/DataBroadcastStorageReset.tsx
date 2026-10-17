import { Badge, Button, Group, Input, Text } from '@mantine/core';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { clearDataBroadcastStorage } from './dataBroadcastStorage';
import { browserStorage } from './volume';

// DataBroadcastStorageReset は、データ放送の文書がこの端末に保存したデータを消すボタン (#502)。郵便番号は残す。
export function DataBroadcastStorageReset() {
  const { t } = useTranslation();
  const [cleared, setCleared] = useState<number | null>(null);

  return (
    <Input.Wrapper
      // 入力欄を持たないので、label 要素にしない。
      labelElement="div"
      label={
        <Group gap={6} component="span">
          {t('settings.account.dataBroadcastStorage')}
          <Badge size="sm" variant="light" color="gray" tt="none" component="span">
            {t('settings.account.deviceOnly')}
          </Badge>
        </Group>
      }
      description={t('settings.account.dataBroadcastStorageHelp')}
    >
      <Group gap="sm" mt={6}>
        <Button
          variant="default"
          size="xs"
          onClick={() => setCleared(clearDataBroadcastStorage(browserStorage()))}
        >
          {t('settings.account.dataBroadcastStorageClear')}
        </Button>
        {cleared !== null && (
          <Text size="sm" c="dimmed" role="status">
            {t('settings.account.dataBroadcastStorageCleared', { count: cleared })}
          </Text>
        )}
      </Group>
    </Input.Wrapper>
  );
}
