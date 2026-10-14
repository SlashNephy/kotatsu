import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  NumberInput,
  Stack,
  Table,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import { AdminPage } from '../../admin/AdminPage';
import { authApi, type OIDCStatus } from '../../auth/api';
import { settingsErrorMessage } from '../../settings/errors';
import { formatBytes } from '../../futon';
import { mutate } from '../../settings/mutate';
import { reauthMethods, useReauthGate } from '../../settings/useReauthGate';
import { MissingRecordings } from '../../admin/MissingRecordings';
import type { systemPoolMutation } from './__generated__/systemPoolMutation.graphql';
import type { systemQuery } from './__generated__/systemQuery.graphql';

const oidcDisabled: OIDCStatus = { enabled: false, displayName: '' };
const path = '/admin/system';

export const Route = createFileRoute('/admin/system')({
  // IdP での再認証を出すかを確かめる。確かめられなくても、ほかの方法で再認証できる。
  loader: () => authApi.oidcStatus().catch(() => oidcDisabled),
  component: SystemPage,
});

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

const formatTime = (time: string) => dateTimeFormat.format(Date.parse(time));

// SystemPage は、システムの状態を見て、保存先プールを変える、管理者向けの画面 (設計書 10.3)。
// チャンネル種別とチャンネルの表示は、チャンネルの表示の画面 (/admin/channels) で変える。
function SystemPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.system.title')}>
      <Stack gap="md">
        <SystemStatus />
        <MissingRecordings />
      </Stack>
    </AdminPage>
  );
}

const query = graphql`
  query systemQuery {
    systemStatus {
      instances {
        instanceId
        role
        address
        startedAt
        heartbeatAt
        alive
      }
      leases {
        name
        holderInstanceId
        generation
        expiresAt
        valid
      }
      tunerServer {
        degraded
        degradedSince
        lastError
        clockSkewMs
        checkedAt
      }
      tuners {
        index
        name
        types
        available
        remote
        fault
      }
      pools {
        id
        path
        priority
        minFreeBytes
        freeBytes
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

function SystemStatus() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const oidc = Route.useLoaderData();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<systemQuery>(
    query,
    {},
    { fetchKey, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const gate = useReauthGate(reauthMethods(data.secondFactors, data.passkeys, oidc), path);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const fail = (caught: unknown) =>
    setMessage({ ok: false, text: settingsErrorMessage(t, caught) });
  const done = () => {
    setMessage({ ok: true, text: t('admin.saved') });
    refresh();
  };
  const { systemStatus: status } = data;

  return (
    <Stack gap="md">
      {gate.modal}
      <Group justify="space-between">
        {message !== null ? (
          <Alert
            color={message.ok ? 'teal' : 'red'}
            role={message.ok ? 'status' : 'alert'}
            withCloseButton
            onClose={() => setMessage(null)}
            flex={1}
          >
            {message.text}
          </Alert>
        ) : (
          <span />
        )}
        <Button size="compact-sm" variant="default" onClick={refresh}>
          {t('admin.system.refresh')}
        </Button>
      </Group>

      <Section title={t('admin.system.tunerServer')}>
        {status.tunerServer == null ? (
          <Text size="sm" c="dimmed">
            {t('admin.system.tunerServerUnchecked')}
          </Text>
        ) : (
          <Stack gap={4}>
            <Group gap="xs">
              <Badge color={status.tunerServer.degraded ? 'red' : 'teal'} variant="light">
                {status.tunerServer.degraded
                  ? t('admin.system.degraded')
                  : t('admin.system.healthy')}
              </Badge>
              {status.tunerServer.degradedSince != null && (
                <Text size="sm">
                  {t('admin.system.degradedSince', {
                    since: formatTime(status.tunerServer.degradedSince),
                  })}
                </Text>
              )}
            </Group>
            {status.tunerServer.lastError !== '' && (
              <Text size="sm" c="red">
                {t('admin.system.lastError', { error: status.tunerServer.lastError })}
              </Text>
            )}
            {status.tunerServer.clockSkewMs != null && (
              <Text size="sm">
                {t('admin.system.clockSkew', { ms: status.tunerServer.clockSkewMs })}
              </Text>
            )}
            <Text size="xs" c="dimmed">
              {t('admin.system.checkedAt', { at: formatTime(status.tunerServer.checkedAt) })}
            </Text>
          </Stack>
        )}
      </Section>

      <Section title={t('admin.system.tuners')}>
        {status.tuners.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('admin.system.noTuners')}
          </Text>
        ) : (
          <DataTable
            head={[
              t('admin.system.tunerIndex'),
              t('admin.system.tunerName'),
              t('admin.system.tunerTypes'),
              t('admin.system.tunerState'),
            ]}
            rows={status.tuners.map((tuner) => ({
              key: String(tuner.index),
              cells: [
                tuner.index,
                tuner.name,
                tuner.types.join(', '),
                <Group gap={4} key="state">
                  <Badge size="sm" variant="light" color={tuner.available ? 'teal' : 'gray'}>
                    {tuner.available
                      ? t('admin.system.tunerAvailable')
                      : t('admin.system.tunerUnavailable')}
                  </Badge>
                  {tuner.remote && (
                    <Badge size="sm" variant="light">
                      {t('admin.system.tunerRemote')}
                    </Badge>
                  )}
                  {tuner.fault && (
                    <Badge size="sm" variant="light" color="red">
                      {t('admin.system.tunerFault')}
                    </Badge>
                  )}
                </Group>,
              ],
            }))}
          />
        )}
      </Section>

      <Section title={t('admin.system.instances')}>
        <DataTable
          head={[
            t('admin.system.instanceRole'),
            t('admin.system.instanceId'),
            t('admin.system.instanceAddress'),
            t('admin.system.instanceStartedAt'),
            t('admin.system.instanceHeartbeatAt'),
            '',
          ]}
          rows={status.instances.map((instance) => ({
            key: `${instance.instanceId}/${instance.role}`,
            cells: [
              instance.role,
              <Text key="id" size="xs" ff="monospace">
                {instance.instanceId}
              </Text>,
              instance.address,
              formatTime(instance.startedAt),
              formatTime(instance.heartbeatAt),
              <Badge key="alive" size="sm" variant="light" color={instance.alive ? 'teal' : 'gray'}>
                {instance.alive ? t('admin.system.alive') : t('admin.system.stopped')}
              </Badge>,
            ],
          }))}
        />
      </Section>

      <Section title={t('admin.system.leases')}>
        <DataTable
          head={[
            t('admin.system.leaseName'),
            t('admin.system.leaseHolder'),
            t('admin.system.leaseGeneration'),
            t('admin.system.leaseExpiresAt'),
            '',
          ]}
          rows={status.leases.map((lease) => ({
            key: lease.name,
            cells: [
              lease.name,
              <Text key="holder" size="xs" ff="monospace">
                {lease.holderInstanceId}
              </Text>,
              lease.generation,
              formatTime(lease.expiresAt),
              <Badge key="valid" size="sm" variant="light" color={lease.valid ? 'teal' : 'gray'}>
                {lease.valid ? t('admin.system.valid') : t('admin.system.expired')}
              </Badge>,
            ],
          }))}
        />
      </Section>

      <Section title={t('admin.system.pools')}>
        {status.pools.length === 0 ? (
          <Text size="sm" c="dimmed">
            {t('admin.system.noPools')}
          </Text>
        ) : (
          status.pools.map((pool) => (
            <PoolEditor
              key={`${pool.id}/${pool.priority}/${pool.minFreeBytes}`}
              pool={pool}
              onSave={(input) =>
                gate.run(async () => {
                  await mutate<systemPoolMutation>(
                    environment,
                    graphql`
                      mutation systemPoolMutation($id: String!, $input: StoragePoolInput!) {
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
    </Stack>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card withBorder>
      <Stack gap="xs">
        <Title order={3} size="h5">
          {title}
        </Title>
        {children}
      </Stack>
    </Card>
  );
}

// DataTable は、狭い画面では横にスクロールする表。
function DataTable({
  head,
  rows,
}: {
  head: string[];
  rows: { key: string; cells: ReactNode[] }[];
}) {
  return (
    <Table.ScrollContainer minWidth={560}>
      <Table
        fz="sm"
        verticalSpacing={4}
        styles={{ td: { whiteSpace: 'nowrap' }, th: { whiteSpace: 'nowrap' } }}
      >
        <Table.Thead>
          <Table.Tr>
            {head.map((label, index) => (
              <Table.Th key={index}>{label}</Table.Th>
            ))}
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {rows.map((row) => (
            <Table.Tr key={row.key}>
              {row.cells.map((cell, index) => (
                <Table.Td key={index}>{cell}</Table.Td>
              ))}
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  );
}

const bytesPerGB = 1e9;

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
