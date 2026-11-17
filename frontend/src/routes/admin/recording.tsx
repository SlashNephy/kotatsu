import { Card, Group, Stack, Text, Title } from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { startTransition, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import { useDataChanges } from '../../changes/DataChanges';
import { AdminPage } from '../../admin/AdminPage';
import { CaptionModels } from '../../admin/CaptionModels';
import { UpdatedAt } from '../../admin/UpdatedAt';
import { FileNameTemplateField } from '../../reservations/FileNameTemplateField';
import {
  frameRateOptions,
  noTranscode,
  optionOfPreset,
  presetOfOption,
  transcodeOptions,
  useTranscodePresets,
} from '../../rules/transcode';
import {
  AutoSaveNumberInput,
  AutoSaveSelect,
  AutoSaveSwitch,
  useAutoSaveField,
} from '../../settings/AutoSaveFields';
import { mutate } from '../../settings/mutate';
import { useAutoSave } from '../../settings/useAutoSave';
import type { recordingMutation } from './__generated__/recordingMutation.graphql';
import type { CMDetector, recordingQuery } from './__generated__/recordingQuery.graphql';

export const Route = createFileRoute('/admin/recording')({
  component: RecordingPage,
});

// autoPool は、保存先プールのメニューで「自動で選ぶ」を表す値。保存するときも空文字のまま送る。
// API では、null は「変えない」で、空文字が「自動で選ぶのに戻す」なので。
const autoPool = '';

// RecordingPage は、録画設定の既定値と、ルールを作るときの既定値を変える、管理者向けの画面 (設計書 4.7、10.3、11.2)。
function RecordingPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.recording.title')}>
      <RecordingSettings />
      {/* 字幕の自動生成のモデル (#1682)。録画の既定値とは別に読み直すので、別の問い合わせにする。 */}
      <CaptionModels />
    </AdminPage>
  );
}

const query = graphql`
  query recordingQuery {
    systemSettings {
      recordingDefaults {
        poolId
        fileNameTemplate
        startMarginSeconds
        endMarginSeconds
        priority
        joinSegments
        detectCm
        generateCaptions
        transcodePreset
        transcodeFrameRate
      }
      ruleDefaults {
        avoidDuplicates
        duplicateWindowDays
        followEventRelay
        bundleSameEpisodes
      }
      ruleUnmatchedGraceMinutes
      tvtplayChaptersEnabled
      cmDetector
      updatedAt
    }
    systemStatus {
      pools {
        id
      }
    }
  }
`;

const saveMutation = graphql`
  mutation recordingMutation($input: SystemSettingsInput!) {
    updateSystemSettings(input: $input) {
      updatedAt
    }
  }
`;

// isCMDetector は、選んだ値が、画面が知っている CM の区間の見分け方 (#1505) かどうかを返す。
function isCMDetector(value: string): value is CMDetector {
  return value === 'FFMPEG' || value === 'LOGO';
}

type Input = recordingMutation['variables']['input'];
type RecordingDefaultsInput = NonNullable<Input['recordingDefaults']>;
type RuleDefaultsInput = NonNullable<Input['ruleDefaults']>;

// RecordingSettings は、録画の既定値を変える。どの項目も、変えたらその項目だけを保存する (#1740)。
// 切り替えと選ぶ項目は変えたらすぐ、数と文字の欄は、欄を離れたときか Enter で保存する。
function RecordingSettings() {
  const { t } = useTranslation();
  const transcodePresets = useTranscodePresets();
  const environment = useRelayEnvironment();
  // fetchKey を変えると、読み直す。読み直す間も画面を出したままにするため、transition の中で変える。
  const [fetchKey, setFetchKey] = useState(0);
  // ほかの管理者が変えたら、読み直す (#869)。
  const adminChanges = useDataChanges(['ADMIN']);
  const data = useLazyLoadQuery<recordingQuery>(
    query,
    {},
    { fetchKey: `${fetchKey}:${adminChanges}`, fetchPolicy: 'store-and-network' },
  );
  const { save } = useAutoSave();
  const settings = data.systemSettings;
  const defaults = settings.recordingDefaults;
  // saveSettings は、変えた項目 input だけを送る。省いた項目は、サーバーが変えない。
  const saveSettings = (input: Input) =>
    save(async () => {
      await mutate<recordingMutation>(environment, saveMutation, { input });
      startTransition(() => setFetchKey((key) => key + 1));
    });
  const saveDefaults = (recordingDefaults: RecordingDefaultsInput) =>
    saveSettings({ recordingDefaults });
  const saveRuleDefaults = (ruleDefaults: RuleDefaultsInput) => saveSettings({ ruleDefaults });
  const template = useAutoSaveField<string, string>({
    value: defaults.fileNameTemplate,
    toDraft: (value) => value,
    parse: (draft) =>
      draft.trim() === '' ? { error: t('settings.autosave.required') } : { value: draft.trim() },
    onSave: (fileNameTemplate) => saveDefaults({ fileNameTemplate }),
  });

  return (
    <Stack gap="md">
      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.settings.recordingDefaults')}
          </Title>
          <Text size="xs" c="dimmed">
            {t('admin.settings.recordingDefaultsNote')}
          </Text>
          <AutoSaveSelect
            size="xs"
            maw={480}
            label={t('admin.settings.pool')}
            description={t('admin.settings.poolHelp')}
            data={[
              { value: autoPool, label: t('admin.settings.poolAuto') },
              ...data.systemStatus.pools.map((pool) => ({ value: pool.id, label: pool.id })),
            ]}
            value={defaults.poolId ?? autoPool}
            onSave={(poolId) => saveDefaults({ poolId })}
          />
          <FileNameTemplateField
            label={t('admin.settings.fileNameTemplate')}
            description={t('admin.settings.fileNameTemplateHelp')}
            value={template.draft}
            onChange={template.setDraft}
            onCommit={template.commit}
            error={template.error}
          />
          <Group gap="xs" align="flex-start">
            <AutoSaveNumberInput
              size="xs"
              w={180}
              label={t('admin.settings.startMargin')}
              min={0}
              max={3600}
              allowDecimal={false}
              value={defaults.startMarginSeconds}
              onSave={(startMarginSeconds) => saveDefaults({ startMarginSeconds })}
            />
            <AutoSaveNumberInput
              size="xs"
              w={180}
              label={t('admin.settings.endMargin')}
              min={0}
              max={3600}
              allowDecimal={false}
              value={defaults.endMarginSeconds}
              onSave={(endMarginSeconds) => saveDefaults({ endMarginSeconds })}
            />
          </Group>
          <AutoSaveNumberInput
            size="xs"
            maw={480}
            label={t('admin.settings.priority')}
            description={t('admin.settings.priorityHelp')}
            min={-10}
            max={10}
            allowDecimal={false}
            value={defaults.priority}
            onSave={(priority) => saveDefaults({ priority })}
          />
          <AutoSaveSwitch
            size="sm"
            label={t('admin.settings.joinSegments')}
            description={t('admin.settings.joinSegmentsHelp')}
            checked={defaults.joinSegments}
            onSave={(joinSegments) => saveDefaults({ joinSegments })}
          />
          <AutoSaveSwitch
            size="sm"
            label={t('admin.settings.detectCm')}
            description={t('admin.settings.detectCmHelp')}
            checked={defaults.detectCm}
            onSave={(detectCm) => saveDefaults({ detectCm })}
          />
          <AutoSaveSelect
            size="sm"
            label={t('admin.settings.cmDetector')}
            description={t('admin.settings.cmDetectorHelp')}
            maw={360}
            data={[
              { value: 'FFMPEG', label: t('admin.settings.cmDetectorFfmpeg') },
              { value: 'LOGO', label: t('admin.settings.cmDetectorLogo') },
            ]}
            value={settings.cmDetector}
            onSave={(value) =>
              isCMDetector(value) ? saveSettings({ cmDetector: value }) : Promise.resolve(false)
            }
          />
          <AutoSaveSwitch
            size="sm"
            label={t('admin.settings.tvtplayChapters')}
            description={t('admin.settings.tvtplayChaptersHelp')}
            checked={settings.tvtplayChaptersEnabled}
            onSave={(tvtplayChaptersEnabled) => saveSettings({ tvtplayChaptersEnabled })}
          />
          <AutoSaveSwitch
            size="sm"
            label={t('admin.settings.generateCaptions')}
            description={t('admin.settings.generateCaptionsHelp')}
            checked={defaults.generateCaptions}
            onSave={(generateCaptions) => saveDefaults({ generateCaptions })}
          />
          <AutoSaveSelect
            size="sm"
            label={t('rules.transcode')}
            description={t('admin.settings.transcodeHelp')}
            maw={360}
            data={transcodeOptions(t, transcodePresets)}
            value={optionOfPreset(defaults.transcodePreset ?? noTranscode)}
            onSave={(value) => saveDefaults({ transcodePreset: presetOfOption(value) })}
          />
          <AutoSaveSelect
            size="sm"
            label={t('rules.transcodeFrameRate')}
            description={t('admin.settings.transcodeFrameRateHelp')}
            maw={360}
            data={frameRateOptions(t)}
            value={defaults.transcodeFrameRate}
            onSave={(transcodeFrameRate) => saveDefaults({ transcodeFrameRate })}
          />
        </Stack>
      </Card>

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.settings.ruleDefaults')}
          </Title>
          <Text size="xs" c="dimmed">
            {t('admin.settings.ruleDefaultsNote')}
          </Text>
          <Group gap="xs" align="flex-end">
            <AutoSaveSwitch
              size="sm"
              label={t('rules.avoidDuplicates')}
              checked={settings.ruleDefaults.avoidDuplicates}
              onSave={(avoidDuplicates) => saveRuleDefaults({ avoidDuplicates })}
            />
            {settings.ruleDefaults.avoidDuplicates && (
              // 空にすると、期間を区切らない。
              <AutoSaveNumberInput
                nullable
                size="xs"
                w={160}
                aria-label={t('rules.duplicateWindowDays')}
                placeholder={t('rules.duplicateWindowDaysPlaceholder')}
                min={1}
                max={3650}
                allowDecimal={false}
                suffix={t('rules.days')}
                value={settings.ruleDefaults.duplicateWindowDays ?? null}
                onSave={(days) =>
                  saveRuleDefaults(
                    days === null
                      ? { clearDuplicateWindowDays: true }
                      : { duplicateWindowDays: days },
                  )
                }
              />
            )}
          </Group>
          <AutoSaveSwitch
            size="sm"
            label={t('rules.followEventRelay')}
            checked={settings.ruleDefaults.followEventRelay}
            onSave={(followEventRelay) => saveRuleDefaults({ followEventRelay })}
          />
          <AutoSaveSwitch
            size="sm"
            label={t('rules.bundleSameEpisodes')}
            checked={settings.ruleDefaults.bundleSameEpisodes}
            onSave={(bundleSameEpisodes) => saveRuleDefaults({ bundleSameEpisodes })}
          />
        </Stack>
      </Card>

      <Card withBorder>
        <Stack gap="xs">
          <Title order={3} size="h5">
            {t('admin.settings.ruleMatching')}
          </Title>
          <AutoSaveNumberInput
            size="xs"
            maw={480}
            label={t('admin.settings.ruleUnmatchedGrace')}
            description={t('admin.settings.ruleUnmatchedGraceHelp')}
            min={0}
            max={1440}
            allowDecimal={false}
            allowNegative={false}
            value={settings.ruleUnmatchedGraceMinutes}
            onSave={(ruleUnmatchedGraceMinutes) => saveSettings({ ruleUnmatchedGraceMinutes })}
          />
        </Stack>
      </Card>

      <UpdatedAt updatedAt={settings.updatedAt} />
    </Stack>
  );
}
