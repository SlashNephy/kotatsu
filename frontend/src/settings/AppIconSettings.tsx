import {
  Badge,
  Card,
  Group,
  Radio,
  SimpleGrid,
  Stack,
  Text,
  Title,
  VisuallyHidden,
} from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { appIconIds, appIconUrls, useAppIcon, type AppIconId } from './appIcon';

// AppIconSettings は、ブラウザのタブとヘッダーに出すアイコンを、この端末で選ぶ設定 (#1665)。
// 選んだらすぐ、タブの favicon とヘッダーのロゴを替える。インストールしたアイコンは変えない。
export function AppIconSettings() {
  const { t } = useTranslation();
  const [current, setCurrent] = useAppIcon();

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Group gap={6}>
          <Title order={3} size="h5">
            {t('settings.appIcon.title')}
          </Title>
          {/* アカウントの設定と並ぶので、この端末だけの設定であることを示す。 */}
          <Badge size="sm" variant="light" color="gray" tt="none">
            {t('settings.account.deviceOnly')}
          </Badge>
        </Group>
        <Text size="sm" c="dimmed">
          {t('settings.appIcon.help')}
        </Text>
        <Radio.Group
          value={current}
          onChange={(value) => setCurrent(value as AppIconId)}
          // 選択肢のまとまりの名前は見出しと同じなので、読み上げにだけ渡す。
          // ラベルは inline-block のままだと、見えなくても 1 行分の高さを取るので、block にする。
          label={<VisuallyHidden>{t('settings.appIcon.title')}</VisuallyHidden>}
          labelProps={{ style: { display: 'block' } }}
        >
          <SimpleGrid cols={{ base: 1, xs: 2, md: 3 }} spacing="xs" verticalSpacing="xs">
            {appIconIds.map((id) => {
              const urls = appIconUrls(id);

              return (
                <Radio.Card key={id} value={id} radius="md" p="sm">
                  <Group gap="sm" wrap="nowrap">
                    <Radio.Indicator />
                    {/* 絵は見本なので、読み上げない。名前は下の文で読む。 */}
                    <img src={urls.app} alt="" width={48} height={48} />
                    <img src={urls.favicon} alt="" width={24} height={24} />
                    <Text size="sm">{t(`settings.appIcon.choices.${id}`)}</Text>
                  </Group>
                </Radio.Card>
              );
            })}
          </SimpleGrid>
        </Radio.Group>
        <Text size="xs" c="dimmed">
          {t('settings.appIcon.installedNote')}
        </Text>
      </Stack>
    </Card>
  );
}
