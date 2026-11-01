import { Alert, Button, Group, Progress, Stack, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { HardDriveDownload, Play, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { formatBytes } from '../futon';
import { detectCapabilities } from '../yunomi/capabilities';
import { offlineSupport, progressRatio } from './offline';
import {
  cancelSave,
  dismissSaveError,
  removeOfflineRecording,
  saveRecording,
  useOfflineRecordings,
  type SaveSource,
} from './store';

type Props = {
  source: SaveSource;
  // videoFormat は、放送の映像の種類。BS4K と 8K は、ブラウザの中で変換できないので保存させない。
  videoFormat: string;
};

// SaveOfflineButton は、録画の詳細に置く「端末に保存」の操作 (#480)。保存している間は進み具合と中止のボタンを、
// 保存した後は端末での再生と、端末から消すボタンを出す。保存しても見られない端末と録画では、何も出さない。
export function SaveOfflineButton({ source, videoFormat }: Props) {
  const support = offlineSupport(globalThis, detectCapabilities().convert, videoFormat);
  if (support !== 'supported') {
    return null;
  }

  return <SaveOffline source={source} />;
}

function SaveOffline({ source }: { source: SaveSource }) {
  const { t } = useTranslation();
  const { saved, states } = useOfflineRecordings();
  const state = states.get(source.id);
  const stored = saved?.find((meta) => meta.id === source.id);

  if (state?.kind === 'saving') {
    const ratio = progressRatio(state.received, state.total);

    return (
      <Stack gap={4}>
        <Group justify="space-between" gap="xs" wrap="nowrap">
          <Text size="sm" role="status">
            {state.total === null
              ? t('offline.saving', { received: formatBytes(state.received) })
              : t('offline.savingOf', {
                  received: formatBytes(state.received),
                  total: formatBytes(state.total),
                })}
          </Text>
          <Button size="compact-sm" variant="default" onClick={() => cancelSave(source.id)}>
            {t('offline.cancel')}
          </Button>
        </Group>
        <Progress
          value={ratio === null ? 100 : ratio * 100}
          animated={ratio === null}
          aria-label={t('offline.progress')}
        />
        <Text size="xs" c="dimmed">
          {t('offline.keepOpen')}
        </Text>
      </Stack>
    );
  }
  if (stored !== undefined) {
    return (
      <Stack gap={4}>
        <Group gap="xs">
          <Button
            variant="light"
            leftSection={<Play size={16} aria-hidden />}
            renderRoot={(props) => (
              <Link {...props} to="/offline/$recordingId" params={{ recordingId: source.id }} />
            )}
          >
            {t('offline.playSaved')}
          </Button>
          <Button
            variant="default"
            color="red"
            leftSection={<Trash2 size={16} aria-hidden />}
            onClick={() => void removeOfflineRecording(source.id)}
          >
            {t('offline.remove')}
          </Button>
        </Group>
        <Text size="xs" c="dimmed">
          {t('offline.savedSize', { size: formatBytes(stored.size) })}
        </Text>
      </Stack>
    );
  }

  return (
    <Stack gap={4}>
      <Button
        variant="default"
        leftSection={<HardDriveDownload size={16} aria-hidden />}
        disabled={saved === null}
        onClick={() => void saveRecording(source)}
      >
        {t('offline.save')}
      </Button>
      {state?.kind === 'error' && (
        <Alert
          color="red"
          role="alert"
          withCloseButton
          onClose={() => dismissSaveError(source.id)}
          closeButtonLabel={t('offline.dismiss')}
        >
          {t(
            state.reason === 'quota'
              ? 'offline.errorQuota'
              : state.reason === 'busy'
                ? 'offline.errorBusy'
                : 'offline.errorFailed',
          )}
        </Alert>
      )}
    </Stack>
  );
}
