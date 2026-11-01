import { Alert, Button, Card, Group, Progress, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { HardDrive, Play, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  EmptyState,
  formatBytes,
  PageLoader,
  ProgramTitle,
  useDocumentTitle,
  zonedFormat,
} from '../../futon';
import { setTimeZone } from '../../futon/timeZone';
import { progressRatio } from '../../offline/offline';
import { cancelSave, removeOfflineRecording, useOfflineRecordings } from '../../offline/store';

export const Route = createFileRoute('/offline/')({
  component: OfflinePage,
});

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// OfflinePage は、端末に保存した録画の一覧 (#480、設計書 7.8)。サーバーにつながらなくても開けるよう、
// GraphQL を読まず、端末に保存した情報だけで描く。サーバーで録画を消しても、ここから消すまでは残す。
function OfflinePage() {
  const { t } = useTranslation();
  useDocumentTitle([t('offline.title')]);
  const { saved, states, persisted } = useOfflineRecordings();
  const estimate = useStorageEstimate(saved);
  // オフラインではサーバーのタイムゾーンを読めないので、保存したときのタイムゾーンで日時を書く。
  const zone = saved?.[0]?.timeZone;
  if (zone !== undefined) {
    setTimeZone(zone);
  }
  const saving = [...states].filter(([, state]) => state.kind === 'saving');

  return (
    <Stack>
      <Title order={2}>{t('offline.title')}</Title>
      <Text size="sm" c="dimmed">
        {t('offline.description')}
      </Text>
      {estimate !== null && estimate.quota !== undefined && (
        <Text size="sm">
          {t('offline.usage', {
            usage: formatBytes(estimate.usage ?? 0),
            quota: formatBytes(estimate.quota),
          })}
        </Text>
      )}
      {persisted === false && <Alert color="amber">{t('offline.notPersisted')}</Alert>}
      {saving.map(([id, state]) =>
        state.kind === 'saving' ? (
          <Card key={id} withBorder>
            <Group justify="space-between" gap="xs" wrap="nowrap">
              <Text size="sm" role="status">
                {state.total === null
                  ? t('offline.saving', { received: formatBytes(state.received) })
                  : t('offline.savingOf', {
                      received: formatBytes(state.received),
                      total: formatBytes(state.total),
                    })}
              </Text>
              <Button size="compact-sm" variant="default" onClick={() => cancelSave(id)}>
                {t('offline.cancel')}
              </Button>
            </Group>
            <Progress
              mt="xs"
              value={(progressRatio(state.received, state.total) ?? 1) * 100}
              aria-label={t('offline.progress')}
            />
          </Card>
        ) : null,
      )}
      {saved === null ? (
        <PageLoader />
      ) : saved.length === 0 ? (
        <EmptyState icon={HardDrive} title={t('offline.empty')} hint={t('offline.emptyHint')} />
      ) : (
        <Stack gap="sm">
          {saved.map((meta) => (
            <Card key={meta.id} withBorder>
              <Group justify="space-between" wrap="wrap" gap="sm">
                <Stack gap={2}>
                  <Text fw={500}>
                    <ProgramTitle name={meta.name} />
                  </Text>
                  <Text size="sm" c="dimmed">
                    {meta.serviceName} · {dateTimeFormat.format(Date.parse(meta.startAt))} ·{' '}
                    {formatBytes(meta.size)}
                  </Text>
                </Stack>
                <Group gap="xs">
                  <Button
                    leftSection={<Play size={16} fill="currentColor" aria-hidden />}
                    renderRoot={(props) => (
                      <Link
                        {...props}
                        to="/offline/$recordingId"
                        params={{ recordingId: meta.id }}
                      />
                    )}
                  >
                    {t('recordings.play')}
                  </Button>
                  <Button
                    variant="default"
                    color="red"
                    leftSection={<Trash2 size={16} aria-hidden />}
                    aria-label={t('offline.removeNamed', { name: meta.name })}
                    onClick={() => void removeOfflineRecording(meta.id)}
                  >
                    {t('offline.remove')}
                  </Button>
                </Group>
              </Group>
            </Card>
          ))}
        </Stack>
      )}
    </Stack>
  );
}

// useStorageEstimate は、端末の使っている量と上限を読む。保存した録画が変わったら読み直す。
function useStorageEstimate(saved: unknown): StorageEstimate | null {
  const [estimate, setEstimate] = useState<StorageEstimate | null>(null);
  useEffect(() => {
    let active = true;
    void navigator.storage
      ?.estimate?.()
      .then((value) => {
        if (active) {
          setEstimate(value);
        }
      })
      .catch(() => {});

    return () => {
      active = false;
    };
  }, [saved]);

  return estimate;
}
