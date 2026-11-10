import {
  Alert,
  Badge,
  Card,
  Group,
  List,
  SimpleGrid,
  Stack,
  Switch,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { ErrorBoundary, formatBytes, PageLoader, useDocumentTitle, zonedFormat } from '../futon';
import { useDataChanges } from '../changes/DataChanges';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { mutate } from '../settings/mutate';
import type {
  networkActivityQuery,
  networkActivityQuery$data,
} from './__generated__/networkActivityQuery.graphql';
import type { networkActivityMutation } from './__generated__/networkActivityMutation.graphql';

export const Route = createFileRoute('/network-activity')({
  component: NetworkActivityPage,
});

type Entry = networkActivityQuery$data['networkActivity']['entries'][number];

// purposeCategories は、通信の目的の分類。題名と一文は、目的ごとの固定の文にし、通信の内容から文を作らない。
const purposeCategories = {
  JIKKYO_CHANNELS: 'jikkyo',
  JIKKYO_COMMENTS: 'jikkyo',
  JIKKYO_KAKOLOG: 'jikkyo',
  NICOLOG_COMMENTS: 'jikkyo',
  ANIME_PROGRAM_SYNC: 'anime',
  ANIME_MAPPINGS: 'anime',
  ANIME_WORK_IDS: 'anime',
  ANIME_ARTWORKS: 'anime',
  ANIME_RECORD: 'anime',
  ANIME_LINK: 'anime',
  ANIME_IMAGE: 'anime',
  USER_AVATAR: 'auth',
  OIDC: 'auth',
  HIBP: 'auth',
  MCP_CLIENT_METADATA: 'auth',
  UPDATE_CHECK: 'operation',
  TELEMETRY_TRACES: 'operation',
  TELEMETRY_METRICS: 'operation',
  WEB_PUSH: 'notification',
  WEBHOOK: 'notification',
  DATA_BROADCAST: 'other',
} as const;

const dayFormat = zonedFormat({ year: 'numeric', month: 'long', day: 'numeric', weekday: 'short' });
const hourFormat = zonedFormat({ hour: '2-digit', minute: '2-digit', hour12: false });
const dateTimeFormat = zonedFormat({ dateStyle: 'medium', timeStyle: 'short' });

// formatTraffic は、小さい通信量も 0 に見えないよう、1 MB 未満は KB で書く。
function formatTraffic(bytes: number): string {
  return bytes < 1024 * 1024 ? `${(bytes / 1024).toFixed(1)} KB` : formatBytes(bytes);
}

// NetworkActivityPage は、サーバーが実際に行った外部との通信を開示する画面 (#1369、設計書 11.8)。
function NetworkActivityPage() {
  const { t } = useTranslation();
  useDocumentTitle([t('networkActivity.title')]);

  return (
    <Stack gap="md">
      <Title order={2}>{t('networkActivity.title')}</Title>
      <Text c="dimmed" size="sm">
        {t('networkActivity.description')}
      </Text>
      <ErrorBoundary fallback={() => <Alert color="red">{t('networkActivity.title')}</Alert>}>
        <Suspense fallback={<PageLoader />}>
          <NetworkActivityContent />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function NetworkActivityContent() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [fetchKey, setFetchKey] = useState(0);
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<networkActivityQuery>(
    graphql`
      query networkActivityQuery {
        viewer {
          role
        }
        networkActivity {
          enabled
          enabledAt
          retentionDays
          totalCount
          totalSentBytes
          totalReceivedBytes
          entries {
            hour
            purpose
            host
            initiator
            success
            count
            sentBytes
            receivedBytes
          }
        }
      }
    `,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const [saving, setSaving] = useState(false);
  const activity = data.networkActivity;

  const setEnabled = (enabled: boolean) => {
    setSaving(true);
    void mutate<networkActivityMutation>(
      environment,
      graphql`
        mutation networkActivityMutation($input: SystemSettingsInput!) {
          updateSystemSettings(input: $input) {
            networkActivityEnabled
          }
        }
      `,
      { input: { networkActivityEnabled: enabled } },
    ).finally(() => {
      setSaving(false);
      setFetchKey((key) => key + 1);
    });
  };

  return (
    <Stack gap="md">
      {data.viewer.role === 'ADMIN' && (
        <Card withBorder>
          <Switch
            label={t('networkActivity.adminLabel')}
            description={t('networkActivity.adminHelp')}
            checked={activity.enabled}
            disabled={saving}
            onChange={(event) => setEnabled(event.currentTarget.checked)}
          />
        </Card>
      )}

      {!activity.enabled && <Alert color="gray">{t('networkActivity.disabled')}</Alert>}

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('networkActivity.totalsTitle', { days: activity.retentionDays })}
          </Title>
          <SimpleGrid cols={3}>
            <Total label={t('networkActivity.totalCount')} value={String(activity.totalCount)} />
            <Total
              label={t('networkActivity.totalSent')}
              value={formatTraffic(activity.totalSentBytes)}
            />
            <Total
              label={t('networkActivity.totalReceived')}
              value={formatTraffic(activity.totalReceivedBytes)}
            />
          </SimpleGrid>
        </Stack>
      </Card>

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('networkActivity.notesTitle')}
          </Title>
          <List size="sm" spacing={4}>
            <List.Item>{t('networkActivity.noteLocal')}</List.Item>
            {activity.enabledAt != null && (
              <List.Item>
                {t('networkActivity.noteSince', {
                  date: dateTimeFormat.format(new Date(activity.enabledAt)),
                })}
              </List.Item>
            )}
            <List.Item>{t('networkActivity.noteAbstract')}</List.Item>
            <List.Item>{t('networkActivity.noteExcluded')}</List.Item>
            <List.Item>{t('networkActivity.noteLost')}</List.Item>
          </List>
        </Stack>
      </Card>

      {activity.enabled && activity.entries.length === 0 && (
        <Text c="dimmed" size="sm">
          {t('networkActivity.empty')}
        </Text>
      )}

      {groupByDay(activity.entries).map(([day, entries]) => (
        <Stack key={day} gap="xs">
          <Title order={3} size="h5">
            {day}
          </Title>
          {entries.map((entry) => (
            <EntryRow
              key={`${entry.hour}/${entry.purpose}/${entry.host ?? ''}/${entry.initiator}/${String(entry.success)}`}
              entry={entry}
            />
          ))}
        </Stack>
      ))}
    </Stack>
  );
}

function Total({ label, value }: { label: string; value: string }) {
  return (
    <Stack gap={0}>
      <Text size="xs" c="dimmed">
        {label}
      </Text>
      <Text fw={600}>{value}</Text>
    </Stack>
  );
}

// groupByDay は、新しい順に並んだ集計を、表示のタイムゾーンの日ごとにまとめる。
function groupByDay(entries: readonly Entry[]): [string, Entry[]][] {
  const days = new Map<string, Entry[]>();
  for (const entry of entries) {
    const day = dayFormat.format(new Date(entry.hour));
    days.set(day, [...(days.get(day) ?? []), entry]);
  }

  return [...days];
}

// Purpose と Initiator は、サーバーが返す固定の種類。Relay の将来の値の分は、スキーマを更新するまで届かない。
type Purpose = keyof typeof purposeCategories;
type Initiator = Exclude<Entry['initiator'], '%future added value'>;

function EntryRow({ entry }: { entry: Entry }) {
  const { t } = useTranslation();
  const purpose = entry.purpose as Purpose;
  const initiator = entry.initiator as Initiator;
  const hour = new Date(entry.hour);
  const until = new Date(hour.getTime() + 60 * 60 * 1000);

  return (
    <Card withBorder padding="sm">
      <Stack gap={4}>
        <Group gap="xs" wrap="wrap">
          <Text size="xs" c="dimmed">
            {hourFormat.format(hour)}〜{hourFormat.format(until)}
          </Text>
          <Badge size="xs" variant="light" tt="none">
            {t(`networkActivity.categories.${purposeCategories[purpose]}`)}
          </Badge>
          <Badge size="xs" variant="default" tt="none">
            {t(`networkActivity.initiator.${initiator}`)}
          </Badge>
          {!entry.success && (
            <Badge size="xs" color="red" variant="light" tt="none">
              {t('networkActivity.failed')}
            </Badge>
          )}
        </Group>
        <Text fw={600} size="sm">
          {t(`networkActivity.purposes.${purpose}.title`)}
        </Text>
        <Text size="sm">{t(`networkActivity.purposes.${purpose}.body`)}</Text>
        <Text size="xs" c="dimmed">
          {entry.host ??
            t(`networkActivity.purposes.${purpose as 'WEBHOOK'}.hostLabel`, {
              defaultValue: t('networkActivity.hostHidden'),
            })}
          ・{t('networkActivity.times', { count: Number(entry.count) })}・ ↑
          {formatTraffic(entry.sentBytes)} ↓{formatTraffic(entry.receivedBytes)}
        </Text>
      </Stack>
    </Card>
  );
}
