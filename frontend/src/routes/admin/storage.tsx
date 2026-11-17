import { Alert, Box, Button, Group, Progress, Stack, Text } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { AdminPage } from '../../admin/AdminPage';
import { useDataChanges } from '../../changes/DataChanges';
import { Section } from '../../admin/Section';
import { AutoSaveNumberInput } from '../../settings/AutoSaveFields';
import { useAutoSave } from '../../settings/useAutoSave';
import { formatBytes } from '../../futon';
import classes from './storage.module.css';
import { mutate } from '../../settings/mutate';
import { MissingRecordings } from '../../admin/MissingRecordings';
import { OrphanFiles } from '../../admin/OrphanFiles';
import { RecordingRenames } from '../../admin/RecordingRenames';
import type {
  StoragePoolInput,
  storagePoolMutation,
} from './__generated__/storagePoolMutation.graphql';
import type { storageQuery } from './__generated__/storageQuery.graphql';

export const Route = createFileRoute('/admin/storage')({
  component: StoragePage,
});

// StoragePage は、保存先プールを変え、録画とファイルを片付ける、管理者向けの画面 (設計書 5.6、10.3)。
function StoragePage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.storage.title')}>
      <StorageSettings />
    </AdminPage>
  );
}

const query = graphql`
  query storageQuery {
    systemStatus {
      pools {
        id
        path
        priority
        minFreeBytes
        freeBytes
        totalBytes
      }
    }
  }
`;

function StorageSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<storageQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const { save } = useAutoSave();
  const { pools } = data.systemStatus;

  return (
    <Stack gap="md">
      {/* 空き容量は保存しなくても変わるので、読み直せるようにする。 */}
      <Group justify="flex-end">
        <Button size="compact-sm" variant="default" onClick={refresh}>
          {t('admin.system.refresh')}
        </Button>
      </Group>

      <Section title={t('admin.system.pools')}>
        {pools.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('admin.system.noPools')}
          </Text>
        ) : (
          pools.map((pool) => (
            <PoolEditor
              key={pool.id}
              pool={pool}
              onSave={(input) =>
                save(async () => {
                  await mutate<storagePoolMutation>(
                    environment,
                    graphql`
                      mutation storagePoolMutation($id: String!, $input: StoragePoolInput!) {
                        updateStoragePool(id: $id, input: $input) {
                          id
                          priority
                          minFreeBytes
                        }
                      }
                    `,
                    { id: pool.id, input },
                  );
                  refresh();
                })
              }
            />
          ))
        )}
      </Section>

      <MissingRecordings />
      <OrphanFiles />
      <RecordingRenames />
    </Stack>
  );
}

const bytesPerGB = 1e9;

// PoolUsage は、保存先プールの使用率のバーと、容量の閾値の位置 (#477)。
// 使用量は、ファイルシステムの容量から、使える空き容量を引いたもの。root のために取っておく分も、使用量に含む。
function PoolUsage({ free, total, minFree }: { free: number; total: number; minFree: number }) {
  const { t } = useTranslation();
  const used = Math.max(0, total - free);
  const percent = (used / total) * 100;
  // 空き容量がこれを下回ると、古い録画から消す。バーの上では、使用量がこの位置を超えたところ。
  const threshold =
    minFree > 0 ? Math.max(0, Math.min(100, ((total - minFree) / total) * 100)) : null;
  const color = free < minFree ? 'red' : percent >= 90 ? 'orange' : 'blue';

  return (
    <Stack gap={2} maw={560}>
      <Box pos="relative">
        <Progress
          value={percent}
          color={color}
          size="lg"
          aria-label={t('admin.system.poolUsageLabel')}
        />
        {threshold !== null && (
          <Box
            className={classes.threshold}
            style={{ left: `${threshold}%` }}
            title={t('admin.system.poolThreshold', { free: formatBytes(minFree) })}
          />
        )}
      </Box>
      <Text size="xs" c="dimmed">
        {t('admin.system.poolUsage', {
          used: formatBytes(used),
          total: formatBytes(total),
          percent: percent.toFixed(1),
        })}
        {threshold !== null &&
          ` ${t('admin.system.poolThreshold', { free: formatBytes(minFree) })}`}
      </Text>
    </Stack>
  );
}

// PoolEditor は、保存先プールの優先順位と容量の閾値を変える。閾値は GB で入力する。
// どちらも、欄を離れたときか Enter で保存する (#1740)。ただし、閾値を今の空き容量より大きくすると、容量の管理が
// 次の確認 (数分ごと) で古い録画を消し始め、取り消せないので、すぐには保存せず、確かめてから保存する。
function PoolEditor({
  pool,
  onSave,
}: {
  pool: {
    id: string;
    path: string;
    priority: number;
    minFreeBytes: number;
    freeBytes: number | null | undefined;
    totalBytes: number | null | undefined;
  };
  onSave: (input: StoragePoolInput) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  // confirming は、確かめてから保存する閾値 (バイト)。
  const [confirming, setConfirming] = useState<number | null>(null);
  // resetKey を変えると、閾値の欄を作り直し、保存してある値に戻す。確かめてやめたときに使う。
  const [resetKey, setResetKey] = useState(0);
  const saveMinFree = (gb: number) => {
    const minFreeBytes = Math.round(gb * bytesPerGB);
    // 空き容量を測れていて、閾値を上げると空き容量が足りなくなるときは、確かめる。
    if (
      pool.freeBytes != null &&
      minFreeBytes > pool.minFreeBytes &&
      minFreeBytes > pool.freeBytes
    ) {
      setConfirming(minFreeBytes);

      return Promise.resolve(false);
    }
    setConfirming(null);

    return onSave({ minFreeBytes });
  };

  return (
    <Stack gap={4} data-pool={pool.id}>
      <Group gap="xs">
        <Text fw={600}>{pool.id}</Text>
        <Text size="xs" c="dimmed">
          {t('admin.system.poolPath', { path: pool.path })}
        </Text>
        <Text size="xs" c="dimmed">
          {pool.freeBytes == null
            ? t('admin.system.poolFreeUnknown')
            : t('admin.system.poolFree', { free: formatBytes(pool.freeBytes) })}
        </Text>
      </Group>
      {pool.freeBytes != null && pool.totalBytes != null && pool.totalBytes > 0 && (
        <PoolUsage free={pool.freeBytes} total={pool.totalBytes} minFree={pool.minFreeBytes} />
      )}
      <Group gap="xs" align="flex-start">
        <AutoSaveNumberInput
          size="xs"
          w={240}
          label={t('admin.system.poolPriority')}
          description={t('admin.system.poolPriorityDescription')}
          min={-1000}
          max={1000}
          allowDecimal={false}
          value={pool.priority}
          onSave={(priority) => onSave({ priority })}
        />
        <AutoSaveNumberInput
          key={resetKey}
          size="xs"
          w={320}
          label={t('admin.system.poolMinFree')}
          description={t('admin.system.poolMinFreeDescription')}
          min={0}
          decimalScale={1}
          value={pool.minFreeBytes / bytesPerGB}
          onSave={saveMinFree}
        />
      </Group>
      {confirming !== null && (
        <Alert color="red" role="alert">
          <Stack gap="xs">
            <Text size="sm">
              {t('admin.system.poolMinFreeConfirm', { free: formatBytes(confirming) })}
            </Text>
            <Group gap="xs">
              <Button
                size="compact-sm"
                color="red"
                onClick={() => {
                  setConfirming(null);
                  void onSave({ minFreeBytes: confirming });
                }}
              >
                {t('admin.system.poolMinFreeConfirmSave')}
              </Button>
              <Button
                size="compact-sm"
                variant="subtle"
                onClick={() => {
                  setConfirming(null);
                  setResetKey((key) => key + 1);
                }}
              >
                {t('admin.system.poolMinFreeConfirmCancel')}
              </Button>
            </Group>
          </Stack>
        </Alert>
      )}
    </Stack>
  );
}
