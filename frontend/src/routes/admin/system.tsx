import { Alert, Anchor, Badge, Button, Group, Stack, Table, Text } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { AdminPage } from '../../admin/AdminPage';
import { Section } from '../../admin/Section';
import { useDataChanges } from '../../changes/DataChanges';
import type { systemQuery } from './__generated__/systemQuery.graphql';
import { zonedFormat } from '../../futon';

export const Route = createFileRoute('/admin/system')({
  component: SystemPage,
});

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

const formatTime = (time: string) => dateTimeFormat.format(Date.parse(time));

// SystemPage は、チューナーサーバー、チューナー、インスタンス、リースの状態を見る、管理者向けの画面 (設計書 10.3)。
// 見るだけの画面なので、本人確認はしない。保存先プールは、保存先の画面 (/admin/storage) で変える。
function SystemPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.system.title')}>
      <SystemStatus />
    </AdminPage>
  );
}

const query = graphql`
  query systemQuery {
    version {
      version
      latestRelease {
        version
        url
        publishedAt
      }
    }
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
        clockSkewed
        clockSkewThresholdMs
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
    }
  }
`;

function SystemStatus() {
  const { t } = useTranslation();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // チューナーとチューナーサーバーの状態が変わったら、読み直す (#867)。
  const tunerChanges = useDataChanges(['TUNERS']);
  const data = useLazyLoadQuery<systemQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${tunerChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const { systemStatus: status } = data;

  return (
    <Stack gap="md">
      <Group justify="flex-end">
        <Button size="compact-sm" variant="default" onClick={refresh}>
          {t('admin.system.refresh')}
        </Button>
      </Group>

      {/* 新しいリリースを確かめる設定を入れていれば、新しい版を知らせる (#506)。 */}
      {data.version.latestRelease != null && (
        <Alert
          color="teal"
          title={t('admin.system.newRelease', { version: data.version.latestRelease.version })}
        >
          <Text size="sm">
            {t('admin.system.newReleaseBody', {
              current: data.version.version,
              published: formatTime(data.version.latestRelease.publishedAt),
            })}{' '}
            <Anchor href={data.version.latestRelease.url} target="_blank" rel="noopener noreferrer">
              {t('admin.system.newReleaseLink')}
            </Anchor>
          </Text>
        </Alert>
      )}

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
              // 閾値を超えたら、警告の色で、閾値とともに示す (#505)。
              <Text
                size="sm"
                c={status.tunerServer.clockSkewed ? 'amber.8' : undefined}
                fw={status.tunerServer.clockSkewed ? 700 : undefined}
              >
                {t(
                  status.tunerServer.clockSkewed
                    ? 'admin.system.clockSkewExceeded'
                    : 'admin.system.clockSkew',
                  {
                    ms: status.tunerServer.clockSkewMs,
                    threshold: status.tunerServer.clockSkewThresholdMs,
                  },
                )}
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
    </Stack>
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
