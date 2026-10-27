import { Alert, Box, Button, Group, NumberInput, Progress, Stack, Text } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { AdminPage } from '../../admin/AdminPage';
import { Section } from '../../admin/Section';
import { authApi, type OIDCStatus } from '../../auth/api';
import { settingsErrorMessage } from '../../settings/errors';
import { formatBytes } from '../../futon';
import classes from './storage.module.css';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import { SudoGate } from '../../settings/SudoGate';
import { MissingRecordings } from '../../admin/MissingRecordings';
import { OrphanFiles } from '../../admin/OrphanFiles';
import { RecordingRenames } from '../../admin/RecordingRenames';
import type { storagePoolMutation } from './__generated__/storagePoolMutation.graphql';
import type { storageQuery } from './__generated__/storageQuery.graphql';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '', linked: false };
const path = '/admin/storage';

export const Route = createFileRoute('/admin/storage')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
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
    secondFactors {
      totp
      remainingRecoveryCodes
    }
    passkeys {
      id
    }
  }
`;

function StorageSettings() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<storageQuery>(
    query,
    {},
    { fetchKey, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const methods = reauthMethods(data.secondFactors, data.passkeys, oidc);
  const gate = useReauthGate(methods, path);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const fail = (caught: unknown) =>
    setMessage({ ok: false, text: settingsErrorMessage(t, caught) });
  const done = () => {
    setMessage({ ok: true, text: t('admin.saved') });
    refresh();
  };
  const { pools } = data.systemStatus;

  return (
    <Stack gap="md">
      {gate.modal}
      <SudoGate methods={methods} returnTo={path}>
        {message !== null && (
          <Alert
            color={message.ok ? 'teal' : 'red'}
            role={message.ok ? 'status' : 'alert'}
            withCloseButton
            onClose={() => setMessage(null)}
          >
            {message.text}
          </Alert>
        )}

        <Section title={t('admin.system.pools')}>
          {pools.length === 0 ? (
            <Text size="sm" c="dimmed">
              {t('admin.system.noPools')}
            </Text>
          ) : (
            pools.map((pool) => (
              <PoolEditor
                key={`${pool.id}/${pool.priority}/${pool.minFreeBytes}`}
                pool={pool}
                onSave={(input) =>
                  gate.run(async () => {
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
                    done();
                  }, fail)
                }
              />
            ))
          )}
        </Section>

        {/* 録画やファイルを消す、動かす操作も重要な操作なので、同じ再認証を通す (設計書 9.5)。 */}
        <MissingRecordings run={gate.run} />
        <OrphanFiles run={gate.run} />
        <RecordingRenames run={gate.run} />
      </SudoGate>
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
  onSave: (input: { priority: number; minFreeBytes: number }) => void;
}) {
  const { t } = useTranslation();
  const [priority, setPriority] = useState<number | string>(pool.priority);
  const [minFreeGB, setMinFreeGB] = useState<number | string>(pool.minFreeBytes / bytesPerGB);

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
      <Group gap="xs" align="flex-end">
        <NumberInput
          size="xs"
          w={240}
          label={t('admin.system.poolPriority')}
          description={t('admin.system.poolPriorityDescription')}
          min={-1000}
          max={1000}
          allowDecimal={false}
          value={priority}
          onChange={setPriority}
        />
        <NumberInput
          size="xs"
          w={320}
          label={t('admin.system.poolMinFree')}
          description={t('admin.system.poolMinFreeDescription')}
          min={0}
          decimalScale={1}
          value={minFreeGB}
          onChange={setMinFreeGB}
        />
        <Button
          size="compact-sm"
          disabled={typeof priority !== 'number' || typeof minFreeGB !== 'number'}
          onClick={() =>
            onSave({
              priority: Number(priority),
              minFreeBytes: Math.round(Number(minFreeGB) * bytesPerGB),
            })
          }
        >
          {t('admin.save')}
        </Button>
      </Group>
    </Stack>
  );
}
