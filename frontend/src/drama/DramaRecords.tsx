import { Alert, Anchor, Button, Card, Group, Stack, Switch, Text, Title } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { animeRecordCallbackError } from '../animeRecords/destinations';
import { useRecordOnDevice } from '../animeRecords/recordOnDevice';
import { useDataChanges } from '../changes/DataChanges';
import { zonedFormat } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { mutate } from '../settings/mutate';
import { AutoSaveNumberInput } from '../settings/AutoSaveFields';
import { useSaveNotice } from '../settings/saveNotice';
import { useAutoSave, useDeviceSave } from '../settings/useAutoSave';
import type { DramaRecordsBeginMutation } from './__generated__/DramaRecordsBeginMutation.graphql';
import type { DramaRecordsDisconnectMutation } from './__generated__/DramaRecordsDisconnectMutation.graphql';
import type { DramaRecordsImportMutation } from './__generated__/DramaRecordsImportMutation.graphql';
import type { DramaRecordsQuery } from './__generated__/DramaRecordsQuery.graphql';
import type { DramaRecordsSettingsMutation } from './__generated__/DramaRecordsSettingsMutation.graphql';

// destinationName は、ドラマの記録先の、画面に出す名前。サービスの名前なので、翻訳しない。
const destinationName = 'Simkl';

const dateFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// DramaRecords は、アカウントの連携の画面で、見たドラマを記録する Simkl との連携を扱う (#1649、設計書 3.9)。
// 管理者が TMDB との連携を無効にしているときは、案内だけを出す。connected と error は、Simkl から戻ってきたときの結果で、
// アニメの記録先と同じ画面に戻るので、ドラマの記録先のもの (simkl、drama_record_) だけを出す。
export function DramaRecords({
  connected,
  error,
  onDismiss,
}: {
  connected: boolean;
  error: string | undefined;
  onDismiss: () => void;
}) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの端末で連携を変えたときと、取り込みが終わったときに、読み直す。
  const changes = useDataChanges(['VIEWER_SETTINGS', 'DRAMA_WATCHLIST']);
  const data = useLazyLoadQuery<DramaRecordsQuery>(
    graphql`
      query DramaRecordsQuery {
        dramaRecordSettings {
          enabled
          thresholdPercent
          destinations {
            destination
            available
            connection {
              accountName
              connectedAt
              reauthorizationRequired
              watchlistImportedAt
              watchlistError
            }
          }
        }
      }
    `,
    {},
    { fetchKey: `${fetchKey}:${changes}`, fetchPolicy: 'store-and-network' },
  );
  const [running, setRunning] = useState(false);
  const [imported, setImported] = useState<boolean | null>(null);
  const notice = useSaveNotice();
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const run = (operation: () => Promise<void>) => {
    setRunning(true);
    operation()
      .catch((caught: unknown) => notice.failed(caught))
      .finally(() => setRunning(false));
  };
  const settings = data.dramaRecordSettings;

  if (!settings.enabled) {
    return (
      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('dramaRecords.title')}
          </Title>
          <Text size="sm" c="dimmed">
            {t('dramaRecords.disabled')}
          </Text>
        </Stack>
      </Card>
    );
  }

  const connect = () =>
    run(async () => {
      const response = await mutate<DramaRecordsBeginMutation>(
        environment,
        graphql`
          mutation DramaRecordsBeginMutation {
            beginDramaRecordConnection(destination: SIMKL)
          }
        `,
        {},
      );
      window.location.assign(response.beginDramaRecordConnection);
    });
  const disconnect = () =>
    run(async () => {
      await mutate<DramaRecordsDisconnectMutation>(
        environment,
        graphql`
          mutation DramaRecordsDisconnectMutation {
            disconnectDramaRecord(destination: SIMKL) {
              enabled
            }
          }
        `,
        {},
      );
      onDismiss();
      refresh();
    });
  const reimport = () =>
    run(async () => {
      const response = await mutate<DramaRecordsImportMutation>(
        environment,
        graphql`
          mutation DramaRecordsImportMutation {
            importDramaWatchlist(force: true)
          }
        `,
        {},
      );
      setImported(response.importDramaWatchlist);
      refresh();
    });
  const callbackError =
    error !== undefined && error.startsWith('drama_record_')
      ? animeRecordCallbackError(error)
      : null;

  return (
    <Stack gap="md">
      <Title order={3} size="h4">
        {t('dramaRecords.title')}
      </Title>
      <Text size="sm" c="dimmed">
        {t('dramaRecords.description')}
      </Text>
      <Text size="sm" c="dimmed">
        {t('dramaRecords.watchlistHint')}{' '}
        <Anchor size="sm" renderRoot={(props) => <Link {...props} to="/drama" />}>
          {t('dramaRecords.watchlistLink')}
        </Anchor>
      </Text>
      <DramaRecordSettings thresholdPercent={settings.thresholdPercent} onSaved={refresh} />
      {callbackError !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={onDismiss}>
          {t(`animeRecords.callbackErrors.${callbackError}`)}
        </Alert>
      )}
      {connected && (
        <Alert color="green" role="status" withCloseButton onClose={onDismiss}>
          {t('dramaRecords.connected', { name: destinationName })}
        </Alert>
      )}
      {settings.destinations.map((status) => {
        const connection = status.connection;

        return (
          <Card key={status.destination} withBorder>
            <Stack gap="xs">
              <Group justify="space-between" wrap="wrap">
                <Title order={4} size="h5">
                  {destinationName}
                </Title>
                <Group gap="xs">
                  {/* 連携し直す必要があるときは、連携し直すことも、外すこともできる。 */}
                  {(connection == null || connection.reauthorizationRequired) && (
                    <Button size="xs" disabled={running || !status.available} onClick={connect}>
                      {t('dramaRecords.connect')}
                    </Button>
                  )}
                  {connection != null && (
                    <Button
                      variant="default"
                      size="xs"
                      color="red"
                      disabled={running}
                      onClick={disconnect}
                    >
                      {t('dramaRecords.disconnect')}
                    </Button>
                  )}
                </Group>
              </Group>
              {connection == null ? (
                <Text size="sm" c="dimmed">
                  {status.available
                    ? t('dramaRecords.notConnected')
                    : t('dramaRecords.notRegistered')}
                </Text>
              ) : (
                <>
                  <Text size="sm">
                    {t('dramaRecords.connectedAs', {
                      account: connection.accountName,
                      date: dateFormat.format(new Date(connection.connectedAt)),
                    })}
                  </Text>
                  {connection.reauthorizationRequired && (
                    <Alert color="amber">{t('dramaRecords.reauthorize')}</Alert>
                  )}
                  <Text size="xs" c="dimmed">
                    {connection.watchlistImportedAt == null
                      ? t('dramaRecords.notImported')
                      : t('dramaRecords.importedAt', {
                          date: dateFormat.format(new Date(connection.watchlistImportedAt)),
                        })}
                  </Text>
                  {connection.watchlistError != null && (
                    <Text size="xs" c="red">
                      {t('dramaRecords.importError', { reason: connection.watchlistError })}
                    </Text>
                  )}
                  {!connection.reauthorizationRequired && (
                    <Group gap="xs">
                      <Button
                        size="compact-xs"
                        variant="default"
                        disabled={running}
                        onClick={reimport}
                      >
                        {t('dramaRecords.reimport')}
                      </Button>
                      {imported !== null && (
                        <Text size="xs" c="dimmed" role="status">
                          {imported
                            ? t('dramaRecords.reimportQueued')
                            : t('dramaRecords.reimportSkipped')}
                        </Text>
                      )}
                    </Group>
                  )}
                </>
              )}
            </Stack>
          </Card>
        );
      })}
    </Stack>
  );
}

// DramaRecordSettings は、見たドラマの話にする割合と、この端末で見たドラマを記録するかを変える (#1649)。見たアニメの設定とは別に持つ。
// Simkl と連携していなくても、Kotatsu の中の見た話 (#1652) に効くので、TMDB との連携が有効なら出す。
// 割合は、アニメの記録の設定と同じく、欄を離れたときか Enter で、1〜100 に収まっていれば保存する (#1740)。
function DramaRecordSettings({
  thresholdPercent,
  onSaved,
}: {
  thresholdPercent: number;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const { save } = useAutoSave();
  const deviceSave = useDeviceSave();
  const [recordOnDevice, setRecordOnDevice] = useRecordOnDevice('drama');

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Title order={4} size="h5">
          {t('dramaRecords.settingsTitle')}
        </Title>
        <Switch
          label={t('dramaRecords.recordOnDevice')}
          description={t('dramaRecords.recordOnDeviceHelp')}
          checked={recordOnDevice}
          onChange={(event) => deviceSave(setRecordOnDevice)(event.currentTarget.checked)}
        />
        <AutoSaveNumberInput
          label={t('dramaRecords.threshold')}
          description={t('dramaRecords.thresholdHelp')}
          min={1}
          max={100}
          allowDecimal={false}
          suffix="%"
          maw={240}
          value={thresholdPercent}
          onSave={(value) =>
            save(async () => {
              await mutate<DramaRecordsSettingsMutation>(
                environment,
                graphql`
                  mutation DramaRecordsSettingsMutation($input: DramaRecordSettingsInput!) {
                    updateDramaRecordSettings(input: $input) {
                      thresholdPercent
                    }
                  }
                `,
                { input: { thresholdPercent: value } },
              );
              onSaved();
            })
          }
        />
      </Stack>
    </Card>
  );
}
