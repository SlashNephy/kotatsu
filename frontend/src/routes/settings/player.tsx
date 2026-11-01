import {
  Alert,
  Anchor,
  Badge,
  Card,
  Group,
  Select,
  Stack,
  Switch,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { ErrorBoundary, PageLoader } from '../../futon';
import { CommentSettingsCard } from '../../jikkyo/CommentSettingsCard';
import { ShortcutKeysCard } from '../../settings/ShortcutKeysCard';
import { isUnauthenticated } from '../../relay/environment';
import {
  autoPictureInPictureSupported,
  useAutoPictureInPicture,
} from '../../yunomi/autoPictureInPicture';
import { captureFileName, captureFileNameMacros } from '../../yunomi/capture';
import { useCaptureFileNameTemplate } from '../../yunomi/captureFileNameTemplate';
import { DataBroadcastStorageReset } from '../../yunomi/DataBroadcastStorageReset';
import { DataBroadcastZipcodeInput } from '../../yunomi/DataBroadcastZipcodeInput';
import { useDataBroadcastPreload } from '../../yunomi/dataBroadcastPreload';
import { useLowLatency } from '../../yunomi/liveCatchUp';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';

// captionBackgrounds は、字幕の背景の選択肢 (#491)。broadcast は放送の指定どおり、ほかは黒の不透明度。
const captionBackgrounds = ['broadcast', '0', '0.25', '0.5', '0.75', '1'];

// captureExampleTime は、キャプチャのファイル名の例に使う時刻。ファイル名は端末のローカル時刻で書くので、ローカル時刻で作る。
const captureExampleTime = new Date(2026, 9, 16, 21, 0, 5).getTime();

export const Route = createFileRoute('/settings/player')({
  component: PlayerSettingsPage,
});

// PlayerSettingsPage は、プレイヤーと実況のコメントの設定 (設計書 10.3、#980)。
function PlayerSettingsPage() {
  const { t } = useTranslation();

  return (
    <Stack gap="md">
      <Anchor component={Link} to="/settings" size="sm">
        {t('settings.back')}
      </Anchor>
      <Title order={2}>{t('settings.pages.player.title')}</Title>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('viewer.error')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <PlayerSettings />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function PlayerSettings() {
  const { t } = useTranslation();
  const player = usePlayerSettings();
  const [dataBroadcastPreload, setDataBroadcastPreload] = useDataBroadcastPreload();
  const [captureTemplate, setCaptureTemplate] = useCaptureFileNameTemplate();
  const [lowLatency, setLowLatency] = useLowLatency();
  const [autoPictureInPicture, setAutoPictureInPicture] = useAutoPictureInPicture();
  // 自動で別のウィンドウに出せないブラウザでは、設定の項目を出さない (#492)。
  const [autoPictureInPictureAvailable] = useState(autoPictureInPictureSupported);

  return (
    <Stack gap="md">
      <Card withBorder>
        <Stack gap="xs">
          <Switch
            label={t('settings.account.jikkyo')}
            checked={player.jikkyoEnabled}
            onChange={(event) => player.setJikkyoEnabled(event.currentTarget.checked)}
          />
          <Switch
            label={t('settings.account.captions')}
            checked={player.captionsEnabled}
            onChange={(event) => player.setCaptionsEnabled(event.currentTarget.checked)}
          />
          {/* 字幕の見た目と、文字スーパーを出すかどうか (#491)。字幕を出さなければ、どれも効かない。 */}
          <Stack gap="xs" pl="lg">
            <Switch
              size="sm"
              label={t('settings.account.superimposeLive')}
              disabled={!player.captionsEnabled}
              checked={player.captions.superimposeLive}
              onChange={(event) =>
                player.setCaptions({
                  ...player.captions,
                  superimposeLive: event.currentTarget.checked,
                })
              }
            />
            <Switch
              size="sm"
              label={t('settings.account.superimposeRecorded')}
              disabled={!player.captionsEnabled}
              checked={player.captions.superimposeRecorded}
              onChange={(event) =>
                player.setCaptions({
                  ...player.captions,
                  superimposeRecorded: event.currentTarget.checked,
                })
              }
            />
            <Switch
              size="sm"
              label={t('settings.account.captionStroke')}
              disabled={!player.captionsEnabled}
              checked={player.captions.stroke}
              onChange={(event) =>
                player.setCaptions({ ...player.captions, stroke: event.currentTarget.checked })
              }
            />
            <Select
              size="xs"
              label={t('settings.account.captionBackground')}
              disabled={!player.captionsEnabled}
              allowDeselect={false}
              data={captionBackgrounds.map((value) => ({
                value,
                label:
                  value === 'broadcast'
                    ? t('settings.account.captionBackgroundBroadcast')
                    : `${Math.round(Number(value) * 100)}%`,
              }))}
              value={
                player.captions.backgroundOpacity === null
                  ? 'broadcast'
                  : String(player.captions.backgroundOpacity)
              }
              onChange={(value) =>
                value !== null &&
                player.setCaptions({
                  ...player.captions,
                  backgroundOpacity: value === 'broadcast' ? null : Number(value),
                })
              }
              maw={240}
            />
          </Stack>
          <Switch
            label={
              <Group gap={6} component="span">
                {t('settings.account.dataBroadcastPreload')}
                {/* アカウントの設定と並ぶので、この端末だけの設定であることを示す。 */}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.dataBroadcastPreloadHelp')}
            checked={dataBroadcastPreload}
            onChange={(event) => setDataBroadcastPreload(event.currentTarget.checked)}
          />
          <Switch
            label={
              <Group gap={6} component="span">
                {t('settings.account.lowLatency')}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.lowLatencyHelp')}
            checked={lowLatency}
            onChange={(event) => setLowLatency(event.currentTarget.checked)}
          />
          <TextInput
            label={
              <Group gap={6} component="span">
                {t('settings.account.captureFileName')}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.captureFileNameHelp', {
              macros: captureFileNameMacros.join(' '),
              example: captureFileName(
                captureExampleTime,
                'NHK総合1・東京',
                'ニュース７🈑',
                captureTemplate,
              ),
            })}
            placeholder={t('settings.account.captureFileNameDefault')}
            value={captureTemplate}
            onChange={(event) => setCaptureTemplate(event.currentTarget.value)}
            maw={480}
          />
          <DataBroadcastZipcodeInput />
          <DataBroadcastStorageReset />
          {autoPictureInPictureAvailable && (
            <Switch
              label={
                <Group gap={6} component="span">
                  {t('settings.account.autoPictureInPicture')}
                  <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                    {t('settings.account.deviceOnly')}
                  </Badge>
                </Group>
              }
              description={t('settings.account.autoPictureInPictureHelp')}
              checked={autoPictureInPicture}
              onChange={(event) => setAutoPictureInPicture(event.currentTarget.checked)}
            />
          )}
        </Stack>
      </Card>

      <ShortcutKeysCard />

      <CommentSettingsCard />
    </Stack>
  );
}
