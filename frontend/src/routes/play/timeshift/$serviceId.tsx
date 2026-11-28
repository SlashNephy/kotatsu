import { Alert, Anchor, Button, Group, Stack, Text } from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Captions, Share2 } from 'lucide-react';
import { Suspense, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../../relay/useLazyLoadQuery';

import { rememberReturnTo } from '../../../auth/returnTo';
import { ErrorBoundary, PageLoader, ProgramTitle } from '../../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../../relay/environment';
import { WatchPanel } from '../../../yunomi/WatchPanel';
import { WatchHeading, WatchLayout, watchCommentsHeight } from '../../../yunomi/WatchLayout';
import { CaptionHistory } from '../../../yunomi/CaptionHistory';
import { StreamInfo, useStreamMeter } from '../../../yunomi/StreamInfo';
import { useDRCSReplacements } from '../../../yunomi/useDRCSReplacements';
import {
  insertRecordedCaption,
  normalizeCaption,
  type CaptionEntry,
} from '../../../yunomi/captionHistory';
import {
  broadcastTime,
  recordingPosition,
  formatClock,
  formatDuration,
} from '../../../yunomi/position';
import { useFastForward } from '../../../yunomi/fastForward';
import {
  RateSelect,
  CropSelect,
  AudioDelaySetting,
  NormalizeVolumeToggle,
  SleepTimerSelect,
  QualitySelect,
} from '../../../yunomi/QualitySelect';
import { hlsURL, toVideoFormat } from '../../../yunomi/route';
import { HlsPlayer } from '../../../yunomi/HlsPlayer';
import { usePlayerAudio } from '../../../yunomi/usePlayerAudio';
import { type PlayerSnapshot } from '../../../yunomi/playerState';
import { SeekBar } from '../../../yunomi/SeekBar';
import { TsPlayer } from '../../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../../yunomi/usePlaybackRoute';
import { useAdaptiveQuality } from '../../../yunomi/useAdaptiveQuality';
import { usePlayerSettings } from '../../../yunomi/usePlayerSettings';
import type { PlayerCommands } from '../../../yunomi/commands';
import { useCapture } from '../../../yunomi/PlayerTools';
import {
  CommandButton,
  PlayButton,
  PlayerControls,
  PositionStatus,
  SeekStepButtons,
  SettingsButton,
  VolumeControl,
} from '../../../yunomi/PlayerControls';
import {
  parseStartSeconds,
  timeshiftShareSeconds,
  timeshiftStartPosition,
} from '../../../yunomi/shareUrl';
import type { ServiceIdPlayTimeshiftQuery } from './__generated__/ServiceIdPlayTimeshiftQuery.graphql';
import { useHostedWatch, useWatchMini } from '../../../yunomi/WatchHost';
import {
  PlayerBox,
  PlayerErrorOverlay,
  PlayerFeedback,
  PlayerFrame,
  PlayerToolButtons,
  usePlayerInputs,
  usePlayerPresence,
  usePlayerShell,
} from '../../../yunomi/playerShell';
import { useVodTimeline } from '../../../yunomi/useVodTimeline';
import { readableParams } from '../../../futon/readableId';

// PlaySearch は、再生の画面の URL のクエリ。program は、再生する番組の始まりの時刻 (RFC 3339)。t は、再生を始める位置 (番組の始まりからの放送の秒)。
type PlaySearch = { program: string; t?: number };

export const Route = createFileRoute('/play/timeshift/$serviceId')({
  // URL には素の ID を出し、画面には GraphQL の ID を渡す (#1346)。
  params: readableParams('serviceId', 'Service'),
  // 右のカラムを置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
  validateSearch: (search: Record<string, unknown>): PlaySearch => {
    const t = parseStartSeconds(search.t);

    return {
      program: typeof search.program === 'string' ? search.program : '',
      ...(t === undefined ? {} : { t }),
    };
  },
  component: PlayPage,
});

// PlayPage は、タイムシフトのバッファにある番組を、頭から再生する画面 (設計書 5.7)。
// バッファは、ブラウザの中で変換する経路だけで再生する。画面は、アプリのルート要素に描く (#1602)。
function PlayPage() {
  const { serviceId } = Route.useParams();
  const { program, t: start } = Route.useSearch();

  return useHostedWatch(<PlayWatch serviceId={serviceId} program={program} start={start} />);
}

// PlayWatch は、サービス serviceId の、program に始まった番組を、位置 start (秒) から再生する画面の内容。
function PlayWatch({
  serviceId,
  program,
  start,
}: {
  serviceId: string;
  program: string;
  start: number | undefined;
}) {
  const { t } = useTranslation();
  // 小さなプレイヤー (#1602) では、戻るリンクを出さず、映像だけにする。
  const mini = useWatchMini();

  return (
    <Stack gap="sm">
      {!mini && (
        <Anchor
          size="sm"
          renderRoot={(props) => (
            <Link {...props} to="/play/live/$serviceId" params={{ serviceId }} />
          )}
        >
          {t('timeshift.backToLive')}
        </Anchor>
      )}
      {/* 同じルートのまま別のサービスや番組へ移ったときは、前の番組のエラーを残さず読み直す (#1453)。 */}
      <ErrorBoundary
        resetKey={`${serviceId}:${program}`}
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor
                component={Link}
                to="/login"
                // 共有された URL をログインしていない端末で開いても、ログインの後に同じ位置から再生する。
                onClick={() => rememberReturnTo(window.location.pathname + window.location.search)}
              >
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">
              {graphQLErrorDetails(error).code === 'NOT_FOUND'
                ? t('timeshift.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Player
            key={`${serviceId}:${program}`}
            serviceId={serviceId}
            startAt={program}
            start={start}
          />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// playQuery は、再生する番組。放送中は、番組の終わりと途切れた区間を知るために読み直す。
const playQuery = graphql`
  query ServiceIdPlayTimeshiftQuery($serviceId: ID!, $startAt: Time!) {
    timeshiftProgram(serviceId: $serviceId, startAt: $startAt) {
      name
      startAt
      endAt
      playableFrom
      gaps {
        from
        to
      }
      streamUrl
      hlsUrl
      service {
        name
        logoUrl
        videoFormat
      }
    }
    transcoding {
      available
      presets {
        name
        codec
        passthrough
        videoBitrate
      }
    }
  }
`;

// refreshMs は、放送中の番組を再生している間に、番組を読み直す間隔。終わった時刻が分かったら、長さを伸ばすのをやめる。
const refreshMs = 30_000;

function Player({
  serviceId,
  startAt,
  start,
}: {
  serviceId: string;
  startAt: string;
  start: number | undefined;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const variables = { serviceId, startAt };
  const { timeshiftProgram: program, transcoding } = useLazyLoadQuery<ServiceIdPlayTimeshiftQuery>(
    playQuery,
    variables,
  );
  // shell は、再生元によらないプレイヤーの外側 (#2244)。
  const shell = usePlayerShell();
  const { mediaRef, media, setMedia, controls, videoState, showFlash } = shell;
  // 録画と同じく、ブラウザの中で変換するか、kyusu の仮想 HLS で再生する (設計書 7.1)。
  // デコードが追いつかずにほかの画面で下げた段階の上限は、段階を下げる放送中の番組でだけ引き継ぐ (#1513)。
  const playback = usePlaybackRoute(transcoding, toVideoFormat(program.service.videoFormat), {
    inheritCap: program.endAt == null,
  });
  const { route, choices, quality, setQuality } = playback;
  const hls = route.kind === 'kyusu';
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    captions: captionSettings,
  } = usePlayerSettings();
  // 放送中なら、番組が終わるまで読み直す。
  const environment = useRelayEnvironment();
  const onAir = program.endAt == null;
  useEffect(() => {
    if (!onAir) {
      return;
    }
    const timer = setInterval(() => {
      fetchQuery<ServiceIdPlayTimeshiftQuery>(environment, playQuery, variables, {
        fetchPolicy: 'network-only',
      })
        .toPromise()
        .catch(() => {});
    }, refreshMs);

    return () => clearInterval(timer);
    // variables は描画ごとに作り直すが、内容は serviceId と startAt だけで決まる。
  }, [environment, onAir, serviceId, startAt]);
  // duration は、再生できる長さ (秒)。途切れた区間の分は、バッファに映像がないので差し引く。放送中は今まで伸ばす。
  const duration = recordingPosition(
    program.playableFrom,
    program.gaps,
    program.endAt == null ? Date.now() : Date.parse(program.endAt),
  );
  // initialPosition は、再生を始める位置 (秒)。URL で位置を渡されたらそこから、なければ再生できる範囲の始まりから。
  const [initialPosition] = useState(() =>
    start === undefined ? 0 : Math.min(timeshiftStartPosition(program, start), duration),
  );
  // stream は、再生できる範囲の始まりからの秒で、位置を指定して読み直す配信 (設計書 7.2)。kyusu の経路では、仮想 HLS を流す。
  const timeline = useVodTimeline({
    streamUrl: program.streamUrl,
    media,
    mediaRef,
    convert: route.kind === 'convert',
    hls,
    initial: initialPosition,
    duration,
  });
  const {
    stream,
    hlsStream,
    position,
    positionRef,
    currentPosition,
    restartAt,
    jumpTo,
    retry,
    error,
  } = timeline;
  const [seeking, setSeeking] = useState<number | null>(null);

  // streamMeter は、右のカラムの「情報」に出す、届いた映像のビットレートと放送の形式を保持する (#828)。
  const streamMeter = useStreamMeter(playback.current);
  // captionEntries は、字幕の履歴 (#450)。字幕が出たときの再生位置で並べ、行を押すとその位置へ移る。
  const [captionEntries, setCaptionEntries] = useState<readonly CaptionEntry[]>([]);
  const currentCaption = useRef<string | null>(null);
  // 字幕の外字を置き換えるテーブルを読む (#825)。
  const drcs = useDRCSReplacements();
  const onCaptionText = (text: string) => {
    const normalized = normalizeCaption(text);
    currentCaption.current = normalized === '' ? null : normalized;
    const at = currentPosition();
    setCaptionEntries((list) => insertRecordedCaption(list, at, text));
  };
  // seek は、指定の位置に移る。再生できる範囲の外へは移らない。
  // kyusu の仮想 HLS は、プレイリストの中を移るだけで、読み直さない。
  const seek = (target: number) => {
    jumpTo(target);
  };
  // changeQuality は、画質を変え、それまでの位置から続ける。
  const changeQuality = (next: string) => {
    restartAt(positionRef.current);
    setQuality(next);
  };
  // 映像だけの遅れを飛び直しで修正する (#1513)。デコードが追いつかなければ、画質が「自動」の間は段階を下げ、それまでの位置から続ける。
  // 段階を下げるのは、放送中の番組 (追っかけと同じく、バッファの端が伸びている) でだけ。
  const adaptive = useAdaptiveQuality(
    media,
    {
      automatic: playback.automatic,
      lowerAvailable: playback.lowerAvailable,
      stepDownForDecode: () => {
        const next = playback.stepDownForDecode();
        if (next !== null) {
          restartAt(positionRef.current);
        }

        return next;
      },
      current: playback.current,
      networkLadder: playback.networkLadder,
      setNetworkStep: (name) => {
        restartAt(positionRef.current);
        playback.setNetworkStep(name);
      },
    },
    // バッファは先に読むので、受け取りの速さで余裕を確認して上げる。
    {
      stepDown: onAir,
      pictureInPicture: shell.pip.container !== null,
      // 放送中の番組は端に追いつくとバッファが伸びるのを待つので、終わりがない。
      network: { mode: 'rate', convert: route.kind === 'convert', topBitrate: null, ends: !onAir },
    },
  );
  // 音声は、経路ごとのメニューで選ぶ。バッファの番組の音声の構成は分からないので、プレイヤーに任せる。
  const audio = usePlayerAudio(route, null, () => restartAt(positionRef.current));
  const [rate, setRate] = useState(1);
  // 早回し (#1572) の間は、選んだ速さより速く再生する。放送中の番組では、バッファの端の手前で元の速さに戻す。
  const fastForward = useFastForward(rate, { growing: onAir, position, duration });
  useEffect(() => {
    if (media !== null) {
      media.defaultPlaybackRate = rate;
      media.playbackRate = fastForward.playbackRate;
    }
  }, [rate, fastForward.playbackRate, media, stream.url, hlsStream.loads]);

  const skip = (seconds: number) => seek(currentPosition() + seconds);
  // 早回しの間は、押している間ずっと、速さを映像の上に出しておく。
  const shownFlash = fastForward.active
    ? t('player.flash.fastForward', { rate: fastForward.playbackRate })
    : shell.flash;
  const capture = useCapture({
    crop: shell.crop,
    frameRef: shell.playerBoxRef,
    media: () => mediaRef.current,
    serviceName: program.service.name,
    programName: program.name,
    captureTime: () => broadcastTime(program.playableFrom, program.gaps, currentPosition()),
    caption: () => currentCaption.current,
  });
  const saveCapture = async () => {
    if ((await capture.capture('save')) === 'saved') {
      showFlash(t('player.flash.captured'));
    }
  };
  // commands は、この画面で今使える操作 (#809)。キーと OS のメディア操作は、どちらもここから呼ぶ。
  const commands: PlayerCommands = {
    ...shell.commands,
    ...shell.seekCommands({
      skip,
      seekTo: seek,
      // 共有する位置は、番組の始まりからの放送の秒にする。
      currentPosition: () => timeshiftShareSeconds(program, currentPosition()),
      retry,
    }),
    toggleCaptions: () => setCaptions(!captions),
    capture: () => void saveCapture(),
    // スリープタイマーと「再生をやめる」のキーは、プレイヤーを閉じて、上の「ライブ視聴に戻る」と同じ画面へ移る (#1426)。
    stop: () => void navigate({ to: '/play/live/$serviceId', params: { serviceId } }),
  };
  // 映像を長押ししたまま横に滑らせて、再生位置を動かす (#1601)。離したら、自分でシークしたものとして扱う。
  const seekLabel = (value: number) =>
    `${formatDuration(value)} (${formatClock(broadcastTime(program.playableFrom, program.gaps, value))})`;
  const inputs = usePlayerInputs(shell, commands, {
    onDoubleTap: (side) => {
      skip(side === 'left' ? -10 : 10);
      showFlash(t(side === 'left' ? 'player.flash.seekBackward' : 'player.flash.seekForward'));
    },
    slideSeek: {
      enabled: duration > 0,
      range: () => ({ position: currentPosition(), duration }),
      onScrub: setSeeking,
      onCommit: (target) => {
        setSeeking(null);
        seek(target);
      },
      onCancel: () => setSeeking(null),
    },
  });
  const { sleep, scrub } = inputs;
  // player は、プレイヤーの今の状態 (#811)。タブの題名と OS のメディア操作は、これを読む。
  // バッファの番組には ID がないので、局と始まりの時刻で見分ける。
  const player = {
    kind: 'TIMESHIFT',
    target: serviceId,
    serviceName: program.service.name,
    program: { id: `${serviceId}:${startAt}`, name: program.name },
    artwork: null,
    paused: videoState.paused,
    position,
    duration,
    playbackRate: fastForward.playbackRate,
    volume: videoState.volume,
    muted: videoState.muted,
    audios: audio.remote.choices,
    audio: audio.remote.selected,
    dataBroadcast: false,
    keyGroups: [],
  } satisfies PlayerSnapshot;
  // タイムシフトの再生は、ほかの端末から操作させない。
  usePlayerPresence(player, commands, null);

  // 番組が終わった位置まで再生したら、次の番組に進まずに止める。
  useEffect(() => {
    if (!onAir && position >= duration && media !== null && !media.paused) {
      media.pause();
    }
  }, [onAir, position, duration, media]);
  const shown = seeking ?? position;
  // gapMarks は、バッファが途切れた位置のマーク。途切れた区間はシークバーの長さに含めないので、長さのないマークにする。
  const gapMarks = program.gaps.map((gap) => {
    const at = recordingPosition(program.playableFrom, program.gaps, Date.parse(gap.from));
    const length = Math.max((Date.parse(gap.to) - Date.parse(gap.from)) / 1000, 0);

    return { from: at, to: at, label: t('player.gapMark', { duration: formatDuration(length) }) };
  });

  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      seek={
        <SeekBar
          bands={gapMarks}
          duration={duration}
          value={shown}
          previewUrl={null}
          label={seekLabel}
          onChange={setSeeking}
          onChangeEnd={(value) => {
            setSeeking(null);
            seek(value);
          }}
        />
      }
      start={
        <>
          <PlayButton video={videoState} commands={commands} />
          <SeekStepButtons commands={commands} onFastForward={fastForward.setHeld} />
          {/* 狭い画面 (スマートフォン) では、再生位置を出す幅を空けるため、ミュートのボタンを外す。本体の音量キーで操作できる。 */}
          <VolumeControl video={videoState} commands={commands} optional />
        </>
      }
      status={
        <PositionStatus
          position={formatDuration(shown)}
          rest={` / ${formatDuration(duration)} · ${formatClock(broadcastTime(program.playableFrom, program.gaps, shown))}`}
        />
      }
      end={
        <>
          <CommandButton
            commands={commands}
            command="toggleCaptions"
            label={t('player.captions')}
            icon={Captions}
            active={captions}
          />
          <SettingsButton>
            <QualitySelect
              choices={choices}
              quality={quality}
              current={playback.current}
              currentCodec={playback.currentCodec}
              onChange={changeQuality}
            />
            {audio.menu}
            <RateSelect value={rate} onChange={setRate} />
            <CropSelect value={shell.crop} onChange={shell.setCrop} />
            <NormalizeVolumeToggle />
            <AudioDelaySetting />
            <SleepTimerSelect timer={sleep} />
          </SettingsButton>
          <PlayerToolButtons shell={shell} commands={commands} capture={capture} />
        </>
      }
    />
  );

  const playerBox = (
    <PlayerBox shell={shell} inputs={inputs}>
      {route.kind === 'convert' && (
        <TsPlayer
          url={stream.url}
          handover={timeline.handover}
          mediaRef={mediaRef}
          onMedia={setMedia}
          controls={controlBar}
          captions={captions}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={shell.shownCrop}
          brightness={inputs.brightness}
          onCaptionText={onCaptionText}
          onLoaded={timeline.onConvertLoaded}
          handoverTarget={stream.handoverTarget}
          onError={timeline.onError}
          onState={timeline.onConvertState}
          onAudio={audio.onAudio}
          onReading={(reading) => {
            streamMeter.onReading(reading);
            adaptive.network.onReading(reading);
          }}
          audio={audio.playerAudio}
          overlay={
            <>
              <PlayerFeedback
                shell={shell}
                flash={shownFlash}
                notice={adaptive.notice}
                scrub={
                  scrub === null
                    ? null
                    : { seconds: scrub, label: seekLabel(scrub), previewUrl: null }
                }
                loadingHidden={error !== null}
              />
              <PlayerErrorOverlay shell={shell} error={error} onRetry={retry} />
            </>
          }
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          // 読み直すたびに URL を変え、hls.js を作り直す。fragment は、サーバーへは送られない。
          url={`${hlsURL(program.hlsUrl, route.preset, audio.kyusuAudio)}#${hlsStream.loads}`}
          mediaRef={mediaRef}
          onMedia={setMedia}
          onSegment={(segment) => {
            streamMeter.onSegment(segment);
            adaptive.network.onSegment(segment);
          }}
          controls={controlBar}
          captions={captions}
          superimpose={captionSettings.superimposeRecorded}
          captionStyle={captionSettings}
          crop={shell.shownCrop}
          brightness={inputs.brightness}
          onCaptionText={onCaptionText}
          live={false}
          startPosition={hlsStream.start}
          overlay={
            <>
              <PlayerFeedback
                shell={shell}
                flash={shownFlash}
                notice={adaptive.notice}
                scrub={
                  scrub === null
                    ? null
                    : { seconds: scrub, label: seekLabel(scrub), previewUrl: null }
                }
                loadingHidden={error !== null}
              />
              <PlayerErrorOverlay shell={shell} error={error} onRetry={retry} />
            </>
          }
          onLoaded={timeline.onHlsLoaded}
          onError={timeline.onError}
        />
      )}
    </PlayerBox>
  );

  return (
    <WatchLayout
      player={
        <PlayerFrame shell={shell} route={route}>
          {playerBox}
        </PlayerFrame>
      }
      info={
        <>
          <WatchHeading
            title={
              program.name === '' ? t('timeshift.untitled') : <ProgramTitle name={program.name} />
            }
            serviceName={program.service.name}
            serviceLogoUrl={program.service.logoUrl}
            time={{
              startAt: program.startAt,
              endAt: program.endAt ?? program.startAt,
              endUndecided: program.endAt == null,
            }}
            actions={
              <Button
                variant="default"
                size="compact-sm"
                leftSection={<Share2 size={14} aria-hidden />}
                onClick={commands.copyPositionUrl}
              >
                {t('player.share')}
              </Button>
            }
          />
          {error !== null && (
            <Alert color="red" role="alert">
              <Group justify="space-between" gap="xs">
                <Text size="sm">{error}</Text>
                <Button size="compact-sm" variant="light" color="red" onClick={retry}>
                  {t('player.retry')}
                </Button>
              </Group>
            </Alert>
          )}
        </>
      }
      side={
        <WatchPanel
          label={t('player.sidePanel')}
          tabs={[
            {
              value: 'captions',
              label: t('captionHistory.tab'),
              content: (
                <CaptionHistory
                  entries={captionEntries}
                  label={(entry) =>
                    formatClock(broadcastTime(program.playableFrom, program.gaps, entry.at))
                  }
                  onSelect={(entry) => seek(entry.at)}
                  canRegisterDRCS={drcs.canRegister}
                  height={watchCommentsHeight}
                />
              ),
            },
            {
              value: 'info',
              label: t('player.streamInfo.tab'),
              content: (
                <StreamInfo
                  videoLag={adaptive.videoLag}
                  network={adaptive.network}
                  meter={streamMeter.meter}
                  media={media}
                  quality={playback.current}
                  kind={route.kind === 'convert' || route.kind === 'kyusu' ? route.kind : null}
                  audios={null}
                  // タイムシフトの再生は、プレイヤーが先まで読むので測れない。平均も分からないので出さない。
                  average={null}
                  height={watchCommentsHeight}
                />
              ),
            },
          ]}
        />
      }
    />
  );
}
