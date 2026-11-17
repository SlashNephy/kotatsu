import { Card, Stack, Title } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { UpdatedAt } from '../../admin/UpdatedAt';
import { AutoSaveNumberInput } from '../../settings/AutoSaveFields';
import { useAutoSave } from '../../settings/useAutoSave';
import { mutate } from '../../settings/mutate';
import type { retentionMutation } from './__generated__/retentionMutation.graphql';
import type { retentionQuery } from './__generated__/retentionQuery.graphql';

export const Route = createFileRoute('/admin/retention')({
  component: RetentionPage,
});

// RetentionPage は、過去の番組と画面内の通知と録画の再生位置を残す日数を変える、管理者向けの画面 (設計書 10.3、11.2)。
function RetentionPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.retention.title')}>
      <RetentionSettings />
    </AdminPage>
  );
}

const query = graphql`
  query retentionQuery {
    systemSettings {
      archiveRetentionDays
      notificationRetentionDays
      watchProgressRetentionDays
      networkActivityRetentionDays
      updatedAt
    }
  }
`;

function RetentionSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<retentionQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const { save } = useAutoSave();
  const settings = data.systemSettings;
  // saveSettings は、変えた項目だけを保存する (#1740)。省いた項目は、サーバーが変えない。
  const saveSettings = (input: retentionMutation['variables']['input']) =>
    save(async () => {
      await mutate<retentionMutation>(
        environment,
        graphql`
          mutation retentionMutation($input: SystemSettingsInput!) {
            updateSystemSettings(input: $input) {
              updatedAt
            }
          }
        `,
        { input },
      );
      startTransition(() => setFetchKey((key) => key + 1));
    });

  // 日数の欄は、欄を離れたときか Enter で、その欄だけを保存する (#1740)。
  return (
    <Stack gap="md">
      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.settings.archive')}
          </Title>
          <AutoSaveNumberInput
            size="xs"
            maw={480}
            label={t('admin.settings.archiveRetentionDays')}
            description={t('admin.settings.archiveRetentionHelp')}
            min={0}
            allowDecimal={false}
            value={settings.archiveRetentionDays}
            onSave={(archiveRetentionDays) => saveSettings({ archiveRetentionDays })}
          />
        </Stack>
      </Card>

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.settings.notifications')}
          </Title>
          <AutoSaveNumberInput
            size="xs"
            maw={480}
            label={t('admin.settings.notificationRetentionDays')}
            description={t('admin.settings.notificationRetentionHelp')}
            min={0}
            allowDecimal={false}
            value={settings.notificationRetentionDays}
            onSave={(notificationRetentionDays) => saveSettings({ notificationRetentionDays })}
          />
        </Stack>
      </Card>

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.settings.watchProgress')}
          </Title>
          <AutoSaveNumberInput
            size="xs"
            maw={480}
            label={t('admin.settings.watchProgressRetentionDays')}
            description={t('admin.settings.watchProgressRetentionHelp')}
            min={0}
            allowDecimal={false}
            value={settings.watchProgressRetentionDays}
            onSave={(watchProgressRetentionDays) => saveSettings({ watchProgressRetentionDays })}
          />
        </Stack>
      </Card>

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.settings.networkActivity')}
          </Title>
          <AutoSaveNumberInput
            size="xs"
            maw={480}
            label={t('admin.settings.networkActivityRetentionDays')}
            description={t('admin.settings.networkActivityRetentionHelp')}
            min={1}
            allowDecimal={false}
            value={settings.networkActivityRetentionDays}
            onSave={(networkActivityRetentionDays) =>
              saveSettings({ networkActivityRetentionDays })
            }
          />
        </Stack>
      </Card>

      <UpdatedAt updatedAt={settings.updatedAt} />
    </Stack>
  );
}
