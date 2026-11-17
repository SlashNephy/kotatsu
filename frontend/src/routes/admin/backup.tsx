import {
  ActionIcon,
  Alert,
  Button,
  Card,
  Code,
  Group,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { Trash2 } from 'lucide-react';
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { UpdatedAt } from '../../admin/UpdatedAt';
import { formatBytes, zonedFormat } from '../../futon';
import { AutoSaveNumberInput } from '../../settings/AutoSaveFields';
import { mutate } from '../../settings/mutate';
import { useAutoSave } from '../../settings/useAutoSave';
import type { backupMutation } from './__generated__/backupMutation.graphql';
import type { backupQuery } from './__generated__/backupQuery.graphql';
import type { backupRunMutation } from './__generated__/backupRunMutation.graphql';

export const Route = createFileRoute('/admin/backup')({
  component: BackupPage,
});

// BackupPage は、DB とキャプチャを定期に書き出すバックアップの、公開鍵と間隔と世代の数を変え、最後の結果を見る、
// 管理者向けの画面 (設計書 10.3、11.4、#1926)。書き出す先のディレクトリは、設定ファイルで決める (設計書 11.2)。
function BackupPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.backup.title')}>
      <BackupSettings />
    </AdminPage>
  );
}

const query = graphql`
  query backupQuery {
    backup {
      settings {
        recipients
        intervalHours
        generations
        updatedAt
      }
      latestRun {
        manual
        startedAt
        finishedAt
        fileName
        size
        error
      }
    }
  }
`;

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// agePublicKey は、age の公開鍵の形 (Bech32 の age1...)。最後の確かめはサーバーが行う。
const agePublicKey = /^age1[02-9ac-hj-np-z]+$/;

function BackupSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたときと、nekonote が書き出し終えたときに、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<backupQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const { save } = useAutoSave();
  const { settings, latestRun } = data.backup;
  const [key, setKey] = useState('');
  // pendingRecipients は、保存を頼んだ公開鍵の一覧。読み直した一覧がこれと同じになるまで、足すのと消すのを止める。
  // 一覧を丸ごと送るので、読み直す前に続けて押すと、前の操作を古い一覧で打ち消してしまうため。
  const [pendingRecipients, setPendingRecipients] = useState<string | null>(null);
  const editingKeys =
    pendingRecipients !== null && pendingRecipients !== settings.recipients.join('\n');
  // requestedAfter は、今すぐ書き出すよう頼んだときの、最後の結果の時刻。結果が変わるまで、頼んだことを出す。
  // 頼んでいなければ undefined。まだ書き出したことがなければ null。
  const [requestedAfter, setRequestedAfter] = useState<string | null | undefined>(undefined);
  const lastFinishedAt = latestRun?.finishedAt ?? null;
  const requested = requestedAfter !== undefined && requestedAfter === lastFinishedAt;
  const keyValid = agePublicKey.test(key) && !settings.recipients.includes(key) && !editingKeys;
  const reload = () => startTransition(() => setFetchKey((value) => value + 1));

  // saveSettings は、変えた項目だけを保存する (#1740)。省いた項目は、サーバーが変えない。
  const saveSettings = (input: backupMutation['variables']['input']) =>
    save(async () => {
      await mutate<backupMutation>(
        environment,
        graphql`
          mutation backupMutation($input: BackupSettingsInput!) {
            updateBackupSettings(input: $input) {
              settings {
                updatedAt
              }
            }
          }
        `,
        { input },
      );
      reload();
    });

  // saveRecipients は、公開鍵の一覧を保存する。保存できなければ、止めていた操作を戻す。
  const saveRecipients = (recipients: string[]) => {
    setPendingRecipients(recipients.join('\n'));

    return saveSettings({ recipients }).then((ok) => {
      if (!ok) {
        setPendingRecipients(null);
      }

      return ok;
    });
  };

  const add = () => {
    if (!keyValid) {
      return;
    }
    void saveRecipients([...settings.recipients, key]).then((ok) => {
      // 足せなかったら、入れた値を残し、やり直せるようにする。
      if (ok) {
        setKey('');
      }
    });
  };

  const run = () => {
    const before = lastFinishedAt;
    void save(async () => {
      await mutate<backupRunMutation>(
        environment,
        graphql`
          mutation backupRunMutation {
            runBackup {
              settings {
                updatedAt
              }
            }
          }
        `,
        {},
      );
      setRequestedAfter(before);
    });
  };

  return (
    <Stack gap="md">
      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.backup.latest')}
          </Title>
          {latestRun == null ? (
            <Text size="sm" c="dimmed">
              {t('admin.backup.never')}
            </Text>
          ) : (
            <Alert color={latestRun.error == null ? 'green' : 'red'} p="xs">
              <Stack gap={4}>
                <Text size="sm">
                  {t(latestRun.error == null ? 'admin.backup.succeeded' : 'admin.backup.failed', {
                    at: dateTimeFormat.format(Date.parse(latestRun.finishedAt)),
                    kind: t(latestRun.manual ? 'admin.backup.manual' : 'admin.backup.scheduled'),
                  })}
                </Text>
                {latestRun.fileName != null && (
                  <Text size="xs">
                    {t('admin.backup.file', {
                      name: latestRun.fileName,
                      size: formatBytes(latestRun.size ?? 0),
                    })}
                  </Text>
                )}
                {latestRun.error != null && <Code block>{latestRun.error}</Code>}
              </Stack>
            </Alert>
          )}
          <Group gap="xs">
            <Button
              size="xs"
              variant="default"
              disabled={settings.recipients.length === 0}
              onClick={run}
            >
              {t('admin.backup.run')}
            </Button>
            <Text size="xs" c="dimmed">
              {settings.recipients.length === 0
                ? t('admin.backup.runNeedsKey')
                : requested
                  ? t('admin.backup.runRequested')
                  : null}
            </Text>
          </Group>
        </Stack>
      </Card>

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.backup.recipients')}
          </Title>
          <Text size="sm" c="dimmed">
            {t('admin.backup.recipientsHelp')}
          </Text>
          {settings.recipients.length === 0 ? (
            <Text size="sm" c="dimmed">
              {t('admin.backup.noRecipients')}
            </Text>
          ) : (
            settings.recipients.map((recipient) => (
              <Group key={recipient} gap="xs" wrap="nowrap">
                <Code fz="xs" style={{ overflowWrap: 'anywhere' }}>
                  {recipient}
                </Code>
                <ActionIcon
                  variant="subtle"
                  color="red"
                  aria-label={t('admin.backup.remove')}
                  disabled={editingKeys}
                  onClick={() =>
                    void saveRecipients(settings.recipients.filter((r) => r !== recipient))
                  }
                >
                  <Trash2 size={16} />
                </ActionIcon>
              </Group>
            ))
          )}
          <Group gap="xs" align="flex-end">
            <TextInput
              size="xs"
              w={480}
              maw="100%"
              label={t('admin.backup.recipientLabel')}
              placeholder="age1..."
              value={key}
              error={
                key !== '' && !agePublicKey.test(key) ? t('admin.backup.recipientInvalid') : null
              }
              onChange={(event) => setKey(event.currentTarget.value.trim())}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  add();
                }
              }}
            />
            <Button size="xs" variant="default" disabled={!keyValid} onClick={add}>
              {t('admin.backup.add')}
            </Button>
          </Group>
        </Stack>
      </Card>

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.backup.schedule')}
          </Title>
          <Text size="sm" c="dimmed">
            {t('admin.backup.scheduleHelp')}
          </Text>
          <AutoSaveNumberInput
            size="xs"
            maw={480}
            label={t('admin.backup.intervalHours')}
            min={1}
            max={720}
            allowDecimal={false}
            allowNegative={false}
            value={settings.intervalHours}
            onSave={(intervalHours) => saveSettings({ intervalHours })}
          />
          <AutoSaveNumberInput
            size="xs"
            maw={480}
            label={t('admin.backup.generations')}
            min={1}
            max={365}
            allowDecimal={false}
            allowNegative={false}
            value={settings.generations}
            onSave={(generations) => saveSettings({ generations })}
          />
        </Stack>
      </Card>

      <UpdatedAt updatedAt={settings.updatedAt} />
    </Stack>
  );
}
