import { Group, Loader, Progress, Stack, Text } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { progressPercent, remainingLabel } from '../admin/jobs';
import { zonedFormat } from '../futon';

// AutoCaptionsState は、録画の字幕の自動生成の状態 (#1682、設計書 6.1)。
export type AutoCaptionsState =
  | 'NOT_GENERATED'
  | 'QUEUED'
  | 'RUNNING'
  | 'GENERATED'
  | 'FAILED'
  | 'NOT_CONFIGURED';

export type AutoCaptionsValue = {
  readonly state: AutoCaptionsState | '%future added value';
  readonly generatedAt: string | null | undefined;
  // model は、最後に生成したときに使ったモデルのファイルの名前。
  readonly model: string | null | undefined;
  // count は、最後に生成した字幕の数。
  readonly count: number;
  readonly progress:
    | { readonly ratio: number; readonly remainingSeconds: number | null | undefined }
    | null
    | undefined;
};

const generatedAtFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// autoCaptionsActive は、生成を待っているか、生成しているかを返す。この間は、CM の区間の見分けと同じく、読み直し続ける。
export function autoCaptionsActive(state: AutoCaptionsValue['state']): boolean {
  return state === 'QUEUED' || state === 'RUNNING';
}

// AutoCaptionsStatus は、録画の字幕の自動生成の状態を出す (#1682)。whisper.cpp とモデルがなくて取りやめたときは、置き方を示す。
export function AutoCaptionsStatus({ status }: { status: AutoCaptionsValue }) {
  const { t } = useTranslation();
  const generatedAt =
    status.generatedAt == null ? null : generatedAtFormat.format(Date.parse(status.generatedAt));

  return (
    <Stack gap={4} role="status">
      {status.state === 'QUEUED' && (
        <Group gap={6}>
          <Loader size={12} aria-hidden />
          <Text size="sm">{t('recordings.autoCaptions.queued')}</Text>
        </Group>
      )}
      {status.state === 'RUNNING' && (
        <>
          <Group gap={6}>
            <Loader size={12} aria-hidden />
            <Text size="sm">{t('recordings.autoCaptions.running')}</Text>
          </Group>
          {status.progress != null && (
            <>
              <Progress
                value={progressPercent(status.progress.ratio)}
                size="sm"
                aria-label={t('recordings.autoCaptions.running')}
              />
              <Text size="xs" c="dimmed">
                {`${progressPercent(status.progress.ratio)}% ・ ${remainingLabel(t, status.progress.remainingSeconds)}`}
              </Text>
            </>
          )}
        </>
      )}
      {status.state === 'GENERATED' && (
        <Text size="sm">
          {status.count > 0
            ? t('recordings.autoCaptions.generated', { count: status.count })
            : t('recordings.autoCaptions.none')}
        </Text>
      )}
      {status.state === 'FAILED' && (
        <Text size="sm" c="red">
          {t('recordings.autoCaptions.failed')}
        </Text>
      )}
      {status.state === 'NOT_CONFIGURED' && (
        <Text size="sm" c="amber.8">
          {t('recordings.autoCaptions.notConfigured')}
        </Text>
      )}
      {(status.state === 'NOT_GENERATED' || status.state === '%future added value') && (
        <Text size="sm" c="dimmed">
          {t('recordings.autoCaptions.notGenerated')}
        </Text>
      )}
      {/* 生成し直している間と、生成し直しに失敗した後は、前に生成した字幕を使っていると添える。 */}
      {generatedAt !== null &&
        (status.state === 'GENERATED' ? (
          <Text size="xs" c="dimmed">
            {t('recordings.autoCaptions.generatedAt', {
              at: generatedAt,
              model: status.model ?? '',
            })}
          </Text>
        ) : (
          <Text size="xs" c="dimmed">
            {t('recordings.autoCaptions.previous', { at: generatedAt, count: status.count })}
          </Text>
        ))}
      {status.count > 0 && (
        <Text size="xs" c="dimmed">
          {t('recordings.autoCaptions.hint')}
        </Text>
      )}
    </Stack>
  );
}
