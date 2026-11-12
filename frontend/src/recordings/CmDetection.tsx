import { Group, Loader, Progress, Stack, Text } from '@mantine/core';
import { startTransition, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { progressPercent, remainingLabel } from '../admin/jobs';
import { zonedFormat } from '../futon';

// CmDetectionState は、録画の CM の区間の見分けの状態 (#1571、設計書 6.1)。
export type CmDetectionState = 'NOT_DETECTED' | 'QUEUED' | 'RUNNING' | 'DETECTED' | 'FAILED';

export type CmDetectionValue = {
  readonly state: CmDetectionState | '%future added value';
  readonly detectedAt: string | null | undefined;
  readonly progress:
    | { readonly ratio: number; readonly remainingSeconds: number | null | undefined }
    | null
    | undefined;
};

// cmPollInterval は、見分けを待っている間と、見分けている間に、状態を読み直す間隔 (ミリ秒)。
// ジョブの画面と同じく、進み具合を書き戻す間隔 (5 秒) に合わせる。
export const cmPollInterval = 5_000;

const detectedAtFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// cmDetectionActive は、見分けを待っているか、見分けているかを返す。
// 終わったことや失敗したことは、録画の変更の知らせ (DataChanges) では届かないことがあるので、この間は読み直し続ける。
export function cmDetectionActive(state: CmDetectionValue['state']): boolean {
  return state === 'QUEUED' || state === 'RUNNING';
}

// useCmDetectionPoll は、active の間、cmPollInterval ごとに増える数と、すぐに増やす関数を返す。
// 数は query の fetchKey に混ぜて、読み直させる。見分けを頼んだ直後は、関数を呼んで、変更の知らせを待たずに読み直す。
export function useCmDetectionPoll(active: boolean): [number, () => void] {
  const [poll, setPoll] = useState(0);
  const refresh = () => startTransition(() => setPoll((value) => value + 1));
  useEffect(() => {
    if (!active) {
      return;
    }
    const timer = window.setInterval(
      () => startTransition(() => setPoll((value) => value + 1)),
      cmPollInterval,
    );

    return () => window.clearInterval(timer);
  }, [active]);

  return [poll, refresh];
}

// CmDetectionStatus は、録画の CM の区間の見分けの状態を出す (#1571)。sections は、いま記録している CM の区間の数。
export function CmDetectionStatus({
  detection,
  sections,
}: {
  detection: CmDetectionValue;
  sections: number;
}) {
  const { t } = useTranslation();
  const detectedAt =
    detection.detectedAt == null ? null : detectedAtFormat.format(Date.parse(detection.detectedAt));

  return (
    <Stack gap={4} role="status">
      {detection.state === 'QUEUED' && (
        <Group gap={6}>
          <Loader size={12} aria-hidden />
          <Text size="sm">{t('recordings.cm.queued')}</Text>
        </Group>
      )}
      {detection.state === 'RUNNING' && (
        <>
          <Group gap={6}>
            <Loader size={12} aria-hidden />
            <Text size="sm">{t('recordings.cm.running')}</Text>
          </Group>
          {detection.progress != null && (
            <>
              <Progress
                value={progressPercent(detection.progress.ratio)}
                size="sm"
                aria-label={t('recordings.cm.running')}
              />
              <Text size="xs" c="dimmed">
                {`${progressPercent(detection.progress.ratio)}% ・ ${remainingLabel(t, detection.progress.remainingSeconds)}`}
              </Text>
            </>
          )}
        </>
      )}
      {detection.state === 'DETECTED' && (
        <Text size="sm">
          {sections > 0 ? t('recordings.cm.found', { count: sections }) : t('recordings.cm.none')}
        </Text>
      )}
      {detection.state === 'FAILED' && (
        <Text size="sm" c="red">
          {t('recordings.cm.failed')}
        </Text>
      )}
      {(detection.state === 'NOT_DETECTED' || detection.state === '%future added value') && (
        <Text size="sm" c="dimmed">
          {t('recordings.cm.notDetected')}
        </Text>
      )}
      {/* 見分け直している間と、見分け直しに失敗した後は、前に見分けた区間を使っていると添える。 */}
      {detectedAt !== null &&
        (detection.state === 'DETECTED' ? (
          <Text size="xs" c="dimmed">
            {t('recordings.cm.detectedAt', { at: detectedAt })}
          </Text>
        ) : (
          <Text size="xs" c="dimmed">
            {t('recordings.cm.previous', { at: detectedAt, count: sections })}
          </Text>
        ))}
    </Stack>
  );
}
