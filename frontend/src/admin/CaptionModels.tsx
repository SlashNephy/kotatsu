import {
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Modal,
  Progress,
  Select,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { startTransition, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { useDataChanges } from '../changes/DataChanges';
import { formatBytes, zonedFormat } from '../futon';
import { captionModelsAnchor } from '../recordings/AutoCaptions';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { settingsErrorMessage } from '../settings/errors';
import { mutate } from '../settings/mutate';
import { progressPercent, remainingLabel } from './jobs';
import { useAction } from './useAction';
import type { CaptionModelsCancelMutation } from './__generated__/CaptionModelsCancelMutation.graphql';
import type { CaptionModelsDeleteMutation } from './__generated__/CaptionModelsDeleteMutation.graphql';
import type { CaptionModelsDownloadMutation } from './__generated__/CaptionModelsDownloadMutation.graphql';
import type {
  CaptionModelKind,
  CaptionModelsQuery,
  CaptionModelsQuery$data,
} from './__generated__/CaptionModelsQuery.graphql';

// refreshInterval は、ダウンロードしている間に読み直す間隔。進み具合は 5 秒ごとに書き戻されるので、それより短くする。
const refreshInterval = 2000;

const query = graphql`
  query CaptionModelsQuery {
    captionModels {
      kind
      state
      overridden
      installed {
        model {
          id
          name
        }
        size
        installedAt
      }
      download {
        model {
          id
          name
        }
        running
        failed
        progress {
          ratio
          remainingSeconds
        }
      }
      models {
        id
        name
        size
        license
        recommended
      }
    }
  }
`;

const downloadMutation = graphql`
  mutation CaptionModelsDownloadMutation($id: String!) {
    downloadCaptionModel(id: $id) {
      kind
      state
    }
  }
`;

const cancelMutation = graphql`
  mutation CaptionModelsCancelMutation($kind: CaptionModelKind!) {
    cancelCaptionModelDownload(kind: $kind) {
      kind
      state
    }
  }
`;

const deleteMutation = graphql`
  mutation CaptionModelsDeleteMutation($kind: CaptionModelKind!) {
    deleteCaptionModel(kind: $kind) {
      kind
      state
    }
  }
`;

type Slot = CaptionModelsQuery$data['captionModels'][number];

const installedAtFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// CaptionModels は、字幕の自動生成に使うモデルを、管理者が選んでダウンロードし、置いたものを確かめ、消す欄 (#1682、設計書 6.1)。
// ダウンロードは nekonote のジョブが行うので、している間は読み直して進み具合を出す。
export function CaptionModels() {
  const { t } = useTranslation();
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者の操作と、ダウンロードを終えたことは、管理のデータの変更として届く。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<CaptionModelsQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  // 録画の詳細の案内から飛んできたときは、読み終えてから、この欄まで送る。読む間は欄がないので、ルーターは送れない。
  useEffect(() => {
    if (window.location.hash === `#${captionModelsAnchor}`) {
      document.getElementById(captionModelsAnchor)?.scrollIntoView();
    }
  }, []);
  const downloading = data.captionModels.some((slot) => slot.state === 'DOWNLOADING');
  useEffect(() => {
    if (!downloading) {
      return;
    }
    const timer = window.setInterval(
      () => startTransition(() => setFetchKey((key) => key + 1)),
      refreshInterval,
    );

    return () => window.clearInterval(timer);
  }, [downloading]);

  return (
    <Card withBorder id={captionModelsAnchor}>
      <Stack gap="sm">
        <Title order={3} size="h5">
          {t('admin.captionModels.title')}
        </Title>
        <Text size="xs" c="dimmed">
          {t('admin.captionModels.description')}
        </Text>
        {data.captionModels.map((slot) => (
          <CaptionModelSlot key={slot.kind} slot={slot} onChanged={refresh} />
        ))}
      </Stack>
    </Card>
  );
}

// defaultModel は、選ぶ欄の最初の値。置いてあるモデルか、勧めるモデル。
function defaultModel(slot: Slot): string | null {
  return (
    slot.installed?.model?.id ??
    slot.models.find((model) => model.recommended)?.id ??
    slot.models[0]?.id ??
    null
  );
}

function CaptionModelSlot({ slot, onChanged }: { slot: Slot; onChanged: () => void }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const { run, busy } = useAction();
  const [selected, setSelected] = useState<string | null>(defaultModel(slot));
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const kind: CaptionModelKind = slot.kind;
  // 文を引く種類。新しい種類が足されたら、書き起こしのモデルの文で出す。
  const textKind = kind === 'VAD' ? 'VAD' : 'WHISPER';
  const operate = (operation: () => Promise<unknown>) =>
    run(
      async () => {
        setError(null);
        await operation();
        onChanged();
      },
      (caught) => setError(settingsErrorMessage(t, caught)),
    );
  const download = () => {
    if (selected === null) {
      return;
    }
    operate(() =>
      mutate<CaptionModelsDownloadMutation>(environment, downloadMutation, { id: selected }),
    );
  };
  const cancel = () =>
    operate(() => mutate<CaptionModelsCancelMutation>(environment, cancelMutation, { kind }));
  const remove = () => {
    setConfirming(false);
    operate(() => mutate<CaptionModelsDeleteMutation>(environment, deleteMutation, { kind }));
  };
  const options = slot.models.map((model) => ({
    value: model.id,
    label: t('admin.captionModels.option', {
      name: model.name,
      size: formatBytes(model.size),
      license: model.license,
      recommended: model.recommended ? t('admin.captionModels.recommended') : '',
    }),
  }));
  const isDownloading = slot.state === 'DOWNLOADING';
  const installed = slot.installed;
  const progress = slot.download?.progress;

  return (
    <Stack gap={6} data-caption-model={kind}>
      <Group gap="xs">
        <Text size="sm" fw={600}>
          {t(`admin.captionModels.kinds.${textKind}`)}
        </Text>
        <Badge size="sm" variant="light" color={stateColor(slot.state)}>
          {t(`admin.captionModels.states.${slot.state as 'NOT_PLACED'}`)}
        </Badge>
      </Group>
      <Text size="xs" c="dimmed">
        {t(`admin.captionModels.kindHelp.${textKind}`)}
      </Text>
      {installed != null && (
        <Text size="sm">
          {installed.model == null
            ? t('admin.captionModels.placedByHand', { size: formatBytes(installed.size) })
            : t('admin.captionModels.placed', {
                name: installed.model.name,
                size: formatBytes(installed.size),
                at:
                  installed.installedAt == null
                    ? ''
                    : installedAtFormat.format(Date.parse(installed.installedAt)),
              })}
        </Text>
      )}
      {slot.overridden && (
        <Alert color="amber" p="xs">
          <Text size="xs">{t(`admin.captionModels.overridden.${textKind}`)}</Text>
        </Alert>
      )}
      {isDownloading && (
        <Stack gap={4} role="status">
          <Text size="sm">
            {slot.download?.running === true
              ? t('admin.captionModels.downloading', { name: slot.download.model?.name ?? '' })
              : t('admin.captionModels.queued', { name: slot.download?.model?.name ?? '' })}
          </Text>
          {progress != null && (
            <>
              <Progress
                value={progressPercent(progress.ratio)}
                size="sm"
                aria-label={t('admin.captionModels.progress')}
              />
              <Text size="xs" c="dimmed">
                {`${progressPercent(progress.ratio)}% ・ ${remainingLabel(t, progress.remainingSeconds)}`}
              </Text>
            </>
          )}
        </Stack>
      )}
      {slot.state === 'FAILED' && (
        <Text size="sm" c="red" role="alert">
          {t('admin.captionModels.failed', { name: slot.download?.model?.name ?? '' })}
        </Text>
      )}
      {error !== null && (
        <Alert color="red" role="alert" p="xs">
          <Text size="xs">{error}</Text>
        </Alert>
      )}
      <Group gap="xs" align="flex-end">
        <Select
          size="xs"
          label={t('admin.captionModels.model')}
          data={options}
          value={selected}
          onChange={setSelected}
          allowDeselect={false}
          disabled={isDownloading || busy}
          style={{ flex: '1 1 16rem' }}
        />
        {isDownloading ? (
          <Button size="xs" variant="default" loading={busy} onClick={cancel}>
            {t('admin.captionModels.cancel')}
          </Button>
        ) : (
          <Button size="xs" loading={busy} disabled={selected === null} onClick={download}>
            {installed == null
              ? t('admin.captionModels.download')
              : t('admin.captionModels.replace')}
          </Button>
        )}
        {installed != null && !isDownloading && (
          <Button
            size="xs"
            variant="default"
            color="red"
            disabled={busy}
            onClick={() => setConfirming(true)}
          >
            {t('admin.captionModels.delete')}
          </Button>
        )}
      </Group>
      <Modal
        opened={confirming}
        onClose={() => setConfirming(false)}
        title={t('admin.captionModels.deleteTitle')}
      >
        <Stack gap="sm">
          <Text size="sm">{t(`admin.captionModels.deleteConfirm.${textKind}`)}</Text>
          <Group justify="flex-end" gap="xs">
            <Button size="xs" variant="default" onClick={() => setConfirming(false)}>
              {t('admin.captionModels.keep')}
            </Button>
            <Button size="xs" color="red" onClick={remove}>
              {t('admin.captionModels.delete')}
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  );
}

function stateColor(state: Slot['state']): string {
  switch (state) {
    case 'PLACED':
      return 'teal';
    case 'DOWNLOADING':
      return 'blue';
    case 'FAILED':
      return 'red';
    default:
      return 'gray';
  }
}
