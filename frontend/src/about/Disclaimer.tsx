import { List, Stack, Text, Title } from '@mantine/core';
import { useTranslation } from 'react-i18next';

// Disclaimer は、免責事項を出す (#1712)。「このアプリについて」と初回セットアップの画面で、同じ文を出す。
// 文は README の「免責事項」の節と同じにする。
export function Disclaimer() {
  const { t } = useTranslation();

  return (
    <Stack gap="xs">
      <Title order={3} size="h5">
        {t('disclaimer.title')}
      </Title>
      <List type="ordered" size="sm" spacing="xs">
        <List.Item>
          <Text size="sm">{t('disclaimer.privateUse')}</Text>
        </List.Item>
        <List.Item>
          <Text size="sm">{t('disclaimer.tuner')}</Text>
        </List.Item>
        <List.Item>
          <Text size="sm">{t('disclaimer.lawful')}</Text>
        </List.Item>
      </List>
    </Stack>
  );
}
