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

import { ErrorBoundary, PageLoader, useDocumentTitle } from '../../futon';
import { CommentSettingsCard } from '../../jikkyo/CommentSettingsCard';
import { ShortcutKeysCard } from '../../settings/ShortcutKeysCard';
import { WatchPositionsClear } from '../../settings/WatchPositionsClear';
import { isUnauthenticated } from '../../relay/environment';
import {
  autoPictureInPictureSupported,
  useAutoPictureInPicture,
} from '../../yunomi/autoPictureInPicture';
import { fullscreenLandscapeSupported, useFullscreenLandscape } from '../../yunomi/landscapeLock';
import { touchSupported, useSwipeControlsSetting } from '../../yunomi/swipeControls';
import { captureFileName, captureFileNameMacros } from '../../yunomi/capture';
import { useCmAutoSkip } from '../../yunomi/cmAutoSkip';
import { useFieldRate } from '../../yunomi/fieldRate';
import { useInterpolation } from '../../yunomi/frameInterpolation';
import { useCaptureFileNameTemplate } from '../../yunomi/captureFileNameTemplate';
import { DataBroadcastStorageReset } from '../../yunomi/DataBroadcastStorageReset';
import { DataBroadcastZipcodeInput } from '../../yunomi/DataBroadcastZipcodeInput';
import { useDataBroadcastPreload } from '../../yunomi/dataBroadcastPreload';
import { useLowLatency } from '../../yunomi/liveCatchUp';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import { useMiniPlayer } from '../../yunomi/miniPlayer';

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
  useDocumentTitle([t('settings.pages.player.title')]);

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
  const [cmAutoSkip, setCmAutoSkip] = useCmAutoSkip();
  const [fieldRate, setFieldRate] = useFieldRate();
  const [interpolation, setInterpolation] = useInterpolation();
  const [autoPictureInPicture, setAutoPictureInPicture] = useAutoPictureInPicture();
  const [miniPlayer, setMiniPlayer] = useMiniPlayer();
  // 自動で別のウィンドウに出せないブラウザでは、設定の項目を出さない (#492)。
  const [autoPictureInPictureAvailable] = useState(autoPictureInPictureSupported);
  const [fullscreenLandscape, setFullscreenLandscape] = useFullscreenLandscape();
  // 画面の向きを固定できないブラウザでは、設定の項目を出さない (#1599)。
  const [fullscreenLandscapeAvailable] = useState(fullscreenLandscapeSupported);
  const [swipeControls, setSwipeControls] = useSwipeControlsSetting();
  // タッチで操作できない端末では、スワイプの設定の項目を出さない (#1598)。
  const [swipeControlsAvailable] = useState(touchSupported);

  return (
    <Stack gap="md">
      <Card withBorder>
        <Stack gap="xs">
          {/* 管理者がオプトインしたソースごとに、ユーザーもオプトインする (#1405、設計書 7.4)。 */}
          {player.availableCommentSources.map((source) => (
            <Switch
              key={source}
              label={t(`settings.account.commentSource.${source}`)}
              checked={player.commentSources.includes(source)}
              onChange={(event) =>
                player.setCommentSources(
                  event.currentTarget.checked
                    ? [...player.commentSources, source]
                    : player.commentSources.filter((enabled) => enabled !== source),
                )
              }
            />
          ))}
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
          <Switch
            label={
              <Group gap={6} component="span">
                {t('settings.account.cmAutoSkip')}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.cmAutoSkipHelp')}
            checked={cmAutoSkip}
            onChange={(event) => setCmAutoSkip(event.currentTarget.checked)}
          />
          {/* 表示が追いつくかは端末によるので、端末ごとに覚える (#1655)。 */}
          <Switch
            label={
              <Group gap={6} component="span">
                {t('settings.account.fieldRate')}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.fieldRateHelp')}
            checked={fieldRate}
            onChange={(event) => setFieldRate(event.currentTarget.checked)}
          />
          {/* GPU の負荷と、作ったコマの崩れ方は端末と映像によるので、端末ごとに覚える (#1655)。 */}
          <Switch
            label={
              <Group gap={6} component="span">
                {t('settings.account.interpolation')}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.interpolationHelp')}
            checked={interpolation}
            onChange={(event) => setInterpolation(event.currentTarget.checked)}
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
          <Switch
            label={
              <Group gap={6} component="span">
                {t('settings.account.miniPlayer')}
                <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                  {t('settings.account.deviceOnly')}
                </Badge>
              </Group>
            }
            description={t('settings.account.miniPlayerHelp')}
            checked={miniPlayer}
            onChange={(event) => setMiniPlayer(event.currentTarget.checked)}
          />
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
          {fullscreenLandscapeAvailable && (
            <Switch
              label={
                <Group gap={6} component="span">
                  {t('settings.account.fullscreenLandscape')}
                  <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                    {t('settings.account.deviceOnly')}
                  </Badge>
                </Group>
              }
              description={t('settings.account.fullscreenLandscapeHelp')}
              checked={fullscreenLandscape}
              onChange={(event) => setFullscreenLandscape(event.currentTarget.checked)}
            />
          )}
          {swipeControlsAvailable && (
            <Switch
              label={
                <Group gap={6} component="span">
                  {t('settings.account.swipeControls')}
                  <Badge size="sm" variant="light" color="gray" tt="none" component="span">
                    {t('settings.account.deviceOnly')}
                  </Badge>
                </Group>
              }
              description={t('settings.account.swipeControlsHelp')}
              checked={swipeControls}
              onChange={(event) => setSwipeControls(event.currentTarget.checked)}
            />
          )}
        </Stack>
      </Card>

      <ShortcutKeysCard />

      {player.jikkyoAvailable && <CommentSettingsCard />}

      <WatchPositionsClear />
    </Stack>
  );
}
