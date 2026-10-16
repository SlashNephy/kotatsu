import { Alert, Anchor, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Captions, MessageSquareText, Tv } from 'lucide-react';
import { startTransition, Suspense, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import {
  ErrorBoundary,
  PageLoader,
  ProgramTitle,
  splitProgramMarks,
  toHalfWidth,
  useDocumentTitle,
} from '../../futon';
import { JikkyoPanel } from '../../jikkyo/JikkyoPanel';
import { useLiveJikkyo } from '../../jikkyo/useLiveJikkyo';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { HlsPlayer } from '../../yunomi/HlsPlayer';
import { PipPlaceholder } from '../../yunomi/PipPlaceholder';
import { DataBroadcastRemote } from '../../yunomi/DataBroadcastRemote';
import {
  BigPlayButton,
  ControlButton,
  FullscreenButton,
  LiveStatus,
  PlayerControls,
  playerControlsClasses,
  PlayerNotice,
  SettingsButton,
  VolumeControl,
} from '../../yunomi/PlayerControls';
import { PlaybackUnavailable, QualitySelect } from '../../yunomi/QualitySelect';
import { reconnectDelay, recordingsThatTookTuner } from '../../yunomi/reconnect';
import { dataBroadcastSource, hlsURL } from '../../yunomi/route';
import { useDataBroadcast } from '../../yunomi/useDataBroadcast';
import { useDataBroadcastPreload } from '../../yunomi/dataBroadcastPreload';
import { usePlayerAudio } from '../../yunomi/usePlayerAudio';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { usePlaybackRoute } from '../../yunomi/usePlaybackRoute';
import { usePlayerSettings } from '../../yunomi/usePlayerSettings';
import { useMediaSession } from '../../yunomi/useMediaSession';
import { usePlayerShortcuts } from '../../yunomi/usePlayerShortcuts';
import { CaptureButton, PipButton } from '../../yunomi/PlayerTools';
import { useControlsVisibility } from '../../yunomi/useControlsVisibility';
import { useFullscreen } from '../../yunomi/useFullscreen';
import { useAutoplayBlocked } from '../../yunomi/useAutoplayBlocked';
import { useVideoState } from '../../yunomi/useVideoState';
import { useWakeLock } from '../../yunomi/useWakeLock';
import { useDocumentPictureInPicture } from '../../yunomi/useDocumentPictureInPicture';
import { useVisible } from '../../yunomi/useVisible';
import type { ServiceIdLiveQuery } from './__generated__/ServiceIdLiveQuery.graphql';
import type { ServiceIdRecordingsQuery } from './__generated__/ServiceIdRecordingsQuery.graphql';
import { ServiceLabel } from '../../guide/ServiceLabel';

export const Route = createFileRoute('/live/$serviceId')({
  component: LiveServicePage,
});

const recordingsQuery = graphql`
  query ServiceIdRecordingsQuery {
    recordings(first: 20, filter: { states: [RECORDING] }) {
      edges {
        node {
          name
          recordedFrom
          service {
            name
            logoUrl
          }
        }
      }
    }
  }
`;

// LiveServicePage は、サービスをライブ視聴する画面。ブラウザの中で変換するか、kyusu の HLS で再生する (設計書 7.1)。
function LiveServicePage() {
  const { t } = useTranslation();
  const { serviceId } = Route.useParams();
  // 放送中の番組を出すので、画面を開いた時刻を、待たない親で決める。
  const [range, setRange] = useState(() => liveRange(Date.now()));
  // 番組が終わったら、次の番組を読み直す。読み直す間も、再生は続ける。
  const refresh = () => startTransition(() => setRange(liveRange(Date.now())));

  return (
    <Stack gap="sm">
      <Anchor component={Link} to="/live" size="sm">
        {t('live.back')}
      </Anchor>
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">
              {graphQLErrorDetails(error).code === 'NOT_FOUND'
                ? t('live.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Live key={serviceId} serviceId={serviceId} range={range} onProgramEnd={refresh} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// liveRange は、now から 1 分の時間帯。ここに重なる番組を、放送中の番組とみなす。
function liveRange(now: number): { from: string; to: string } {
  return { from: new Date(now).toISOString(), to: new Date(now + 60_000).toISOString() };
}

// undecidedRecheckMs は、終了時刻が未定の番組や、番組がないときに、放送中の番組を読み直す間隔。
const undecidedRecheckMs = 60_000;

// Disconnection は、視聴が切れたことと、つなぎ直すまでの秒と、切れた理由とみなす録画。
type Disconnection = {
  at: number;
  retryIn: number;
  recordings: { name: string; serviceName: string }[];
};

function Live({
  serviceId,
  range,
  onProgramEnd,
}: {
  serviceId: string;
  range: { from: string; to: string };
  onProgramEnd: () => void;
}) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const data = useLazyLoadQuery<ServiceIdLiveQuery>(
    graphql`
      query ServiceIdLiveQuery($id: ID!, $from: Time!, $to: Time!) {
        node(id: $id) {
          __typename
          ... on Service {
            name
            logoUrl
            streamUrl
            jikkyo {
              id
              name
            }
          }
        }
        guide(from: $from, to: $to, services: [$id]) {
          programs {
            name
            endAt
            endUndecided
            audios {
              componentType
              languages
            }
          }
        }
        transcoding {
          available
          presets {
            name
            codec
          }
        }
      }
    `,
    { id: serviceId, ...range },
  );
  const videoRef = useRef<HTMLVideoElement | null>(null);
  // 画質を変えると video の要素を作り直すので、見えているかは、変わらない外側の要素で調べる。
  const frameRef = useRef<HTMLDivElement | null>(null);
  const inPage = useVisible(frameRef);
  // pip は、視聴画面を出す別のウィンドウ。出している間は、このページが隠れても、実況のコメントなどを描き続ける (設計書 7.5)。
  const pip = useDocumentPictureInPicture();
  const visible = inPage || pip.container !== null;
  // playerBoxRef は、プレイヤーを包む要素。別のウィンドウに出すと移るので、キャプチャと全画面は、これを使う。
  const playerBoxRef = useRef<HTMLDivElement | null>(null);
  // video は、今の video の要素。画質を変えたときや、別のウィンドウに移したときに作り直される。
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const videoState = useVideoState(video);
  // ブラウザに音ありの自動再生を断られたら、映像の真ん中に再生ボタンを出す。
  const autoplay = useAutoplayBlocked(video);
  const fullscreen = useFullscreen(playerBoxRef);
  const controls = useControlsVisibility({ playing: !videoState.paused });
  // 再生している間は、画面を消さない。
  useWakeLock(!videoState.paused);
  const { route, choices, quality, setQuality } = usePlaybackRoute(data.transcoding);
  const {
    captionsEnabled: captions,
    setCaptionsEnabled: setCaptions,
    jikkyoEnabled,
    setJikkyoEnabled,
  } = usePlayerSettings();
  const channel = data.node?.__typename === 'Service' ? (data.node.jikkyo ?? null) : null;
  const jikkyo = useLiveJikkyo(jikkyoEnabled ? (channel?.id ?? null) : null, videoRef, visible);
  // dataBroadcastEnabled は、データ放送を出すかどうか。映像を覆うので、既定では出さない (設計書 7.4)。
  const [dataBroadcastEnabled, setDataBroadcastEnabled] = useState(false);
  const [dataBroadcastPreload] = useDataBroadcastPreload();
  // dataBroadcastUsed は、このページでデータ放送を出したことがあるか。一度出したら、消しても受け取りを続け、次はすぐに出す。
  const [dataBroadcastUsed, setDataBroadcastUsed] = useState(false);
  if (dataBroadcastEnabled && !dataBroadcastUsed) {
    setDataBroadcastUsed(true);
  }
  // session は、つなぎ直すたびに増やし、URL を変えて読み直させる。
  const [session, setSession] = useState(0);
  const attempt = useRef(0);
  // preempted は、録画のためにチューナーを使われて切れたかどうか。つなぎ直せない間の理由に使う。
  const preempted = useRef(false);
  const [disconnection, setDisconnection] = useState<Disconnection | null>(null);

  // 視聴が切れたら、理由を調べ、間隔を延ばしながらつなぎ直す。
  const disconnected = () => {
    if (disconnection !== null) {
      return;
    }
    const at = Date.now();
    // 録画にチューナーを使われて切れた後は、録画が続いている間はつなぎ直せない。
    const retried = attempt.current > 0 && preempted.current;
    const delay = reconnectDelay(attempt.current);
    setDisconnection({ at, retryIn: delay, recordings: [] });
    void fetchQuery<ServiceIdRecordingsQuery>(
      environment,
      recordingsQuery,
      {},
      {
        fetchPolicy: 'network-only',
      },
    )
      .toPromise()
      .then((result) => {
        const inProgress =
          result?.recordings.edges.map((edge) => ({
            name: edge.node.name,
            recordedFrom: edge.node.recordedFrom,
            serviceName: edge.node.service.name,
          })) ?? [];
        // 最初に切れたときは、その前後に始まった録画を理由とみなす。つなぎ直せなかったときは、
        // 録画が続いている間はチューナーが空かないので、録画中のものをすべて示す。
        const recordings = retried ? inProgress : recordingsThatTookTuner(inProgress, at);
        if (recordings.length > 0) {
          preempted.current = true;
        }
        setDisconnection((prev) => (prev?.at === at ? { ...prev, recordings } : prev));
      })
      // 理由を調べられなくても、つなぎ直しは続ける。
      .catch(() => {});
  };

  // つなぎ直すまでを数える。
  useEffect(() => {
    if (disconnection === null) {
      return;
    }
    if (disconnection.retryIn <= 0) {
      attempt.current += 1;
      setDisconnection(null);
      setSession((n) => n + 1);

      return;
    }
    const timer = setTimeout(
      () =>
        setDisconnection((prev) => (prev === null ? null : { ...prev, retryIn: prev.retryIn - 1 })),
      1000,
    );

    return () => clearTimeout(timer);
  }, [disconnection]);

  const program = data.guide[0]?.programs[0] ?? null;
  // 番組が終わる時刻に、放送中の番組を読み直す。終了時刻が未定か、番組がなければ、1 分ごとに読み直す。
  const programEnd = program === null || program.endUndecided ? null : program.endAt;
  useEffect(() => {
    const delay = programEnd === null ? undecidedRecheckMs : Date.parse(programEnd) - Date.now();
    const timer = setTimeout(onProgramEnd, Math.max(delay, 1000));

    return () => clearTimeout(timer);
  }, [programEnd, range, onProgramEnd]);
  // 音声は、経路ごとのメニューで選び、画質を変えても同じ音声を保つ。kyusu の経路では、番組の音声の構成から選ぶ。
  const audio = usePlayerAudio(route, program?.audios ?? null);
  const programName = program?.name ?? null;
  const serviceName = data.node?.__typename === 'Service' ? data.node.name : '';
  const serviceLogoUrl = data.node?.__typename === 'Service' ? (data.node.logoUrl ?? null) : null;
  usePlayerShortcuts({
    togglePlay: () => {
      const video = videoRef.current;
      if (video?.paused === true) {
        void video.play().catch(() => {});
      } else {
        video?.pause();
      }
    },
    toggleCaptions: () => setCaptions(!captions),
    toggleMute: videoState.toggleMute,
    toggleFullscreen: fullscreen.toggle,
  });
  useDocumentTitle([programName, serviceName]);
  useMediaSession(
    { title: programName ?? serviceName, artist: serviceName, artwork: serviceLogoUrl },
    {
      play: () => void videoRef.current?.play().catch(() => {}),
      pause: () => videoRef.current?.pause(),
    },
  );

  const service = data.node?.__typename === 'Service' ? data.node : null;
  // データ放送は、プリロードする設定か、一度出したあとは、出していない間も裏で受け取り、出したらすぐに d ボタンで入れるようにする
  // (設計書 7.4)。
  // 視聴が切れている間と、映像が見えていない間は、データ放送の TS も読まない。
  const dataBroadcast = useDataBroadcast(
    service !== null &&
      (dataBroadcastPreload || dataBroadcastUsed) &&
      visible &&
      disconnection === null
      ? dataBroadcastSource(service.streamUrl)
      : null,
    videoRef,
    dataBroadcastEnabled,
  );
  if (service === null) {
    return <Alert color="amber">{t('live.notFound')}</Alert>;
  }
  // session を付けて、つなぎ直すたびに URL を変える。
  const withSession = (base: string) =>
    `${base}${base.includes('?') ? '&' : '?'}session=${session}`;
  const onLoaded = () => {
    attempt.current = 0;
    preempted.current = false;
  };

  const controlBar = (
    <PlayerControls
      visible={controls.visible}
      start={<VolumeControl video={videoState} />}
      status={<LiveStatus />}
      end={
        <>
          <ControlButton
            label={t('player.captions')}
            icon={Captions}
            active={captions}
            onClick={() => setCaptions(!captions)}
          />
          {channel !== null && (
            <ControlButton
              label={t('jikkyo.toggle')}
              icon={MessageSquareText}
              active={jikkyoEnabled}
              onClick={() => setJikkyoEnabled(!jikkyoEnabled)}
            />
          )}
          <ControlButton
            label={t('player.dataBroadcast')}
            icon={Tv}
            active={dataBroadcastEnabled}
            optional
            onClick={() => setDataBroadcastEnabled(!dataBroadcastEnabled)}
          />
          {(choices.length >= 2 || audio.available) && (
            <SettingsButton>
              <QualitySelect choices={choices} quality={quality} onChange={setQuality} />
              {audio.menu}
            </SettingsButton>
          )}
          <CaptureButton
            frameRef={playerBoxRef}
            videoRef={videoRef}
            serviceName={service.name}
            programName={programName}
            captureTime={Date.now}
          />
          <PipButton pip={pip} />
          <FullscreenButton active={fullscreen.active} onToggle={fullscreen.toggle} />
        </>
      }
    />
  );

  const playerBox = (
    <div
      ref={playerBoxRef}
      className={playerControlsClasses.box}
      data-controls-visible={controls.visible}
      {...controls.containerProps}
    >
      {route.kind === 'convert' && (
        <TsPlayer
          url={disconnection === null ? withSession(service.streamUrl) : null}
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          onLoaded={onLoaded}
          overlay={
            <>
              {dataBroadcast.layer}
              {jikkyo.overlay}
              {dataBroadcast.receiving && (
                <PlayerNotice loading>{t('player.dataBroadcastReceiving')}</PlayerNotice>
              )}
              {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
            </>
          }
          onState={(state) => {
            // ライブのストリームは終わらないので、読み終えたのは、切れたということ。
            if (state === 'completed' || state === 'error') {
              disconnected();
            }
          }}
          onError={disconnected}
          onAudio={audio.onAudio}
          audio={audio.convertAudio}
        />
      )}
      {route.kind === 'kyusu' && (
        <HlsPlayer
          url={
            disconnection === null
              ? withSession(hlsURL(service.streamUrl, route.preset, audio.kyusuAudio))
              : null
          }
          videoRef={videoRef}
          onVideo={setVideo}
          controls={controlBar}
          captions={captions}
          live
          onLoaded={onLoaded}
          overlay={
            <>
              {dataBroadcast.layer}
              {jikkyo.overlay}
              {dataBroadcast.receiving && (
                <PlayerNotice loading>{t('player.dataBroadcastReceiving')}</PlayerNotice>
              )}
              {autoplay.blocked && <BigPlayButton onPlay={autoplay.play} />}
            </>
          }
          // チューナーを使われて切れると、hls.js が読み直しを諦めたところで、致命的なエラーになる。
          onError={disconnected}
        />
      )}
    </div>
  );

  return (
    <Stack gap="xs">
      <Title order={2} size="h4">
        <ServiceLabel name={service.name} logoUrl={service.logoUrl} />
      </Title>
      <Text size="sm" c="dimmed">
        {programName === null ? t('live.noProgram') : <ProgramTitle name={programName} />}
      </Text>
      <PlaybackUnavailable route={route} />
      {/* 画面の外に出たときと、タブが隠れたときも再生は続け、データ放送の受信と、実況のコメントの描画だけを止める (設計書 7.2)。 */}
      <div ref={frameRef}>
        {pip.container === null ? playerBox : <PipPlaceholder onBack={pip.close} />}
      </div>
      {pip.container !== null && createPortal(playerBox, pip.container)}
      {disconnection !== null && (
        <Alert color="amber" role="alert" title={t('live.disconnected')}>
          <Stack gap={4}>
            {disconnection.recordings.length > 0 ? (
              <Text size="sm">
                {t('live.tunerTaken', {
                  recordings: disconnection.recordings
                    .map((recording) =>
                      toHalfWidth(
                        `${recording.serviceName}「${splitProgramMarks(recording.name).name}」`,
                      ),
                    )
                    .join('、'),
                })}
              </Text>
            ) : (
              <Text size="sm">{t('live.streamEnded')}</Text>
            )}
            <Text size="sm" c="dimmed">
              {t('live.retryIn', { seconds: disconnection.retryIn })}
            </Text>
          </Stack>
        </Alert>
      )}
      {dataBroadcastEnabled && (
        <DataBroadcastRemote
          state={dataBroadcast.state}
          receiving={dataBroadcast.receiving}
          onPress={dataBroadcast.press}
        />
      )}
      {channel !== null && jikkyoEnabled && (
        <JikkyoPanel channelName={channel.name} status={jikkyo.status} onPost={jikkyo.post} />
      )}
    </Stack>
  );
}
