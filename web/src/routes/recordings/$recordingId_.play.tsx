import { ActionIcon, Alert, Anchor, Group, Slider, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { formatDuration, playbackPosition } from '../../yunomi/position';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { useVisible } from '../../yunomi/useVisible';
import type { RecordingIdPlayQuery } from './__generated__/RecordingIdPlayQuery.graphql';

export const Route = createFileRoute('/recordings/$recordingId_/play')({
  component: PlayPage,
});

// PlayPage は、録画をブラウザの中で変換して再生する画面 (設計書 7.2)。
function PlayPage() {
  const { t } = useTranslation();
  const { recordingId } = Route.useParams();

  return (
    <Stack gap="sm">
      <Anchor
        size="sm"
        renderRoot={(props) => (
          <Link {...props} to="/recordings/$recordingId" params={{ recordingId }} />
        )}
      >
        {t('player.backToRecording')}
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
                ? t('recordings.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Player key={recordingId} recordingId={recordingId} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Player({ recordingId }: { recordingId: string }) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<RecordingIdPlayQuery>(
    graphql`
      query RecordingIdPlayQuery($id: ID!) {
        node(id: $id) {
          __typename
          ... on Recording {
            name
            streamUrl
            recordedFrom
            recordedUntil
            service {
              name
            }
          }
        }
      }
    `,
    { id: recordingId },
  );
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const visible = useVisible(videoRef);
  // offset は、今の読み込みを始めた、録画の始まりからの秒。シークすると、新しい開始位置で読み直す (設計書 7.2)。
  const [offset, setOffset] = useState(0);
  // base は、読み込み直後の video の currentTime。変換した映像の時刻は 0 から始まるとは限らない。
  const base = useRef(0);
  const [position, setPosition] = useState(0);
  const [seeking, setSeeking] = useState<number | null>(null);
  const [paused, setPaused] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // wasPlaying は、画面の外に出たときに再生していたかどうか。戻ったら再生を続ける。
  const wasPlaying = useRef(false);

  const recording = data.node?.__typename === 'Recording' ? data.node : null;
  const duration =
    recording === null
      ? 0
      : ((recording.recordedUntil == null ? Date.now() : Date.parse(recording.recordedUntil)) -
          Date.parse(recording.recordedFrom)) /
        1000;

  // 画面の外に出たら止め、戻ったら続ける。
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return;
    }
    if (!visible) {
      wasPlaying.current = !video.paused;
      video.pause();
    } else if (wasPlaying.current) {
      void video.play().catch(() => {});
    }
  }, [visible]);

  // 再生位置を、録画の始まりからの秒で追う。
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return;
    }
    const update = () => setPosition(playbackPosition(offset, base.current, video.currentTime));
    const onPause = () => setPaused(true);
    const onPlay = () => setPaused(false);
    video.addEventListener('timeupdate', update);
    video.addEventListener('pause', onPause);
    video.addEventListener('play', onPlay);

    return () => {
      video.removeEventListener('timeupdate', update);
      video.removeEventListener('pause', onPause);
      video.removeEventListener('play', onPlay);
    };
  }, [offset]);

  if (recording === null) {
    return <Alert color="amber">{t('recordings.notFound')}</Alert>;
  }
  const url = `${recording.streamUrl}?start=${offset.toFixed(1)}`;
  const shown = seeking ?? position;

  return (
    <Stack gap="xs">
      <Title order={2} size="h4">
        {recording.name}
      </Title>
      <Text size="sm" c="dimmed">
        {recording.service.name}
      </Text>
      <TsPlayer
        url={url}
        videoRef={videoRef}
        active
        onLoaded={() => {
          const video = videoRef.current;
          base.current = video !== null && video.buffered.length > 0 ? video.buffered.start(0) : 0;
          setError(null);
        }}
        onError={() => setError(t('player.error'))}
      />
      <Group gap="sm" wrap="nowrap">
        <ActionIcon
          variant="default"
          size="lg"
          aria-label={paused ? t('player.play') : t('player.pause')}
          onClick={() => {
            const video = videoRef.current;
            if (video === null) {
              return;
            }
            if (video.paused) {
              void video.play().catch(() => {});
            } else {
              video.pause();
            }
          }}
        >
          {paused ? '▶' : '❚❚'}
        </ActionIcon>
        <Slider
          aria-label={t('player.seek')}
          style={{ flex: 1 }}
          min={0}
          max={Math.max(duration, 1)}
          step={1}
          value={Math.min(shown, duration)}
          label={(value) => formatDuration(value)}
          onChange={setSeeking}
          onChangeEnd={(value) => {
            setSeeking(null);
            setPosition(value);
            setOffset(value);
          }}
        />
        <Text size="sm" ff="monospace" style={{ flexShrink: 0 }}>
          {formatDuration(shown)} / {formatDuration(duration)}
        </Text>
      </Group>
      {error !== null && (
        <Alert color="red" role="alert">
          {error}
        </Alert>
      )}
    </Stack>
  );
}
