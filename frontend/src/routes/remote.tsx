import {
  ActionIcon,
  Alert,
  Badge,
  Button,
  Card,
  Group,
  Loader,
  SegmentedControl,
  Slider,
  Stack,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import {
  Pause,
  Play,
  Rewind,
  FastForward,
  Square,
  Tv,
  Volume2,
  VolumeX,
  MonitorSmartphone,
} from 'lucide-react';
import { Suspense, startTransition, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment, useSubscription } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { EmptyState, ErrorBoundary, PageLoader, ProgramTitle, useDocumentTitle } from '../futon';
import { estimatedPosition, remoteKeyName } from '../remote/remote';
import { RemoteOpen } from '../remote/RemoteOpen';
import { settingsErrorMessage } from '../settings/errors';
import { deviceName } from '../settings/device';
import { mutate } from '../settings/mutate';
import { DataBroadcastRemote } from '../yunomi/DataBroadcastRemote';
import { formatDuration } from '../yunomi/position';
import { RemoteKey, type KeyGroup } from '../yunomi/useDataBroadcast';
import type { remoteChangedSubscription } from './__generated__/remoteChangedSubscription.graphql';
import type {
  PlayerCommandInput,
  remoteCommandMutation,
} from './__generated__/remoteCommandMutation.graphql';
import type { remoteQuery, remoteQuery$data } from './__generated__/remoteQuery.graphql';

export const Route = createFileRoute('/remote')({
  component: RemotePage,
});

// refreshMs は、選んだプレイヤーの再生位置を読み直す間隔。プレイヤーは状態を 10 秒ごとに送る。
const refreshMs = 10_000;
// tickMs は、再生位置の見積もりを描き直す間隔。
const tickMs = 1000;
// skipSeconds は、早送りと巻き戻しで移る秒数。
const skipSeconds = 30;

// RemotePage は、同じアカウントのほかの端末で再生しているプレイヤーを操作するリモコン (#459、設計書 7.7)。
// スマートフォンから、テレビにつないだ PC などのプレイヤーを操作する使い方を想定する。
function RemotePage() {
  const { t } = useTranslation();
  useDocumentTitle([t('remote.title')]);

  return (
    <Stack gap="md" maw={560}>
      <Title order={2}>{t('remote.title')}</Title>
      <ErrorBoundary fallback={() => <Alert color="amber">{t('remote.error')}</Alert>}>
        <Suspense fallback={<PageLoader />}>
          <RemoteControl />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

const query = graphql`
  query remoteQuery {
    players {
      id
      current
      userAgent
      lastSeenAt
      state {
        kind
        title
        paused
        position
        duration
        volume
        muted
        audios {
          id
          label
        }
        audio
        dataBroadcast
        keyGroups
      }
    }
  }
`;

type Player = remoteQuery$data['players'][number];

function RemoteControl() {
  const { t } = useTranslation();
  // version は、プレイヤーの一覧の版。変わったら読み直す。tick は、選んだプレイヤーの再生位置を読み直すための数。
  const [version, setVersion] = useState(0);
  const [tick, setTick] = useState(0);
  const config = useMemo(
    () => ({
      subscription: graphql`
        subscription remoteChangedSubscription {
          playersChanged
        }
      `,
      variables: {},
      onNext: (response: remoteChangedSubscription['response'] | null | undefined) => {
        if (response) {
          startTransition(() => setVersion(response.playersChanged));
        }
      },
      onError: () => {},
    }),
    [],
  );
  useSubscription<remoteChangedSubscription>(config);
  useEffect(() => {
    const timer = setInterval(() => startTransition(() => setTick((n) => n + 1)), refreshMs);

    return () => clearInterval(timer);
  }, []);
  const data = useLazyLoadQuery<remoteQuery>(
    query,
    {},
    { fetchKey: `${version}:${tick}`, fetchPolicy: 'store-and-network' },
  );
  // 自分の端末のプレイヤーは、操作する相手にしない。
  const players = data.players.filter((player) => !player.current);
  const [selected, setSelected] = useState<string | null>(null);
  const player = players.find((candidate) => candidate.id === selected) ?? players[0] ?? null;

  if (player === null) {
    return (
      <EmptyState icon={MonitorSmartphone} title={t('remote.empty')} hint={t('remote.emptyHint')} />
    );
  }

  return (
    <Stack gap="md">
      {players.length > 1 && (
        <SegmentedControl
          fullWidth
          value={player.id}
          onChange={setSelected}
          data={players.map((candidate) => ({
            value: candidate.id,
            label: deviceName(candidate.userAgent) ?? t('remote.unknownDevice'),
          }))}
        />
      )}
      <PlayerRemote key={player.id} player={player} />
    </Stack>
  );
}

function PlayerRemote({ player }: { player: Player }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [error, setError] = useState<string | null>(null);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), tickMs);

    return () => clearInterval(timer);
  }, []);
  const { state } = player;
  const send = (command: PlayerCommandInput) => {
    setError(null);
    mutate<remoteCommandMutation>(
      environment,
      graphql`
        mutation remoteCommandMutation($id: ID!, $command: PlayerCommandInput!) {
          sendPlayerCommand(id: $id, command: $command)
        }
      `,
      { id: player.id, command },
    ).catch((caught: unknown) => setError(settingsErrorMessage(t, caught)));
  };
  const position = estimatedPosition(state, player.lastSeenAt, now);
  // seeking と volume は、スライダーを動かしている間の値。離したところで送り、ほかの端末から新しい状態が届くまで保つ。
  // すぐに捨てると、届くまでの間、スライダーが前の値に戻って見える。
  const [seeking, setSeeking] = useState<number | null>(null);
  const [volume, setVolume] = useState<number | null>(null);
  useEffect(() => {
    setSeeking(null);
  }, [player.lastSeenAt]);
  useEffect(() => {
    setVolume(null);
  }, [state.volume]);
  const keyGroups = new Set(state.keyGroups as KeyGroup[]);

  return (
    <Card withBorder>
      <Stack gap="md">
        {error !== null && (
          <Alert color="red" role="alert" withCloseButton onClose={() => setError(null)}>
            {error}
          </Alert>
        )}
        <Stack gap={4}>
          <Group gap="xs">
            <Badge variant="light">
              {t(state.kind === 'LIVE' ? 'remote.kindLive' : 'remote.kindRecording')}
            </Badge>
            <Text size="sm" c="dimmed">
              {deviceName(player.userAgent) ?? t('remote.unknownDevice')}
            </Text>
          </Group>
          {state.title != null && (
            <Text fw={600}>
              <ProgramTitle name={state.title} />
            </Text>
          )}
        </Stack>

        {state.kind === 'RECORDING' && position !== null && state.duration != null && (
          <Stack gap={4}>
            <Slider
              thumbLabel={t('remote.position')}
              min={0}
              max={state.duration}
              step={1}
              value={seeking ?? position}
              label={(value) => formatDuration(value)}
              onChange={setSeeking}
              onChangeEnd={(value) => send({ type: 'SEEK', position: value })}
            />
            <Group justify="space-between">
              <Text size="xs" c="dimmed">
                {formatDuration(seeking ?? position)}
              </Text>
              <Text size="xs" c="dimmed">
                {formatDuration(state.duration)}
              </Text>
            </Group>
          </Stack>
        )}

        <Group justify="center" gap="lg">
          {state.kind === 'RECORDING' && position !== null && (
            <ActionIcon
              variant="default"
              size="xl"
              radius="xl"
              aria-label={t('remote.rewind', { seconds: skipSeconds })}
              onClick={() => send({ type: 'SEEK', position: Math.max(0, position - skipSeconds) })}
            >
              <Rewind size={22} aria-hidden />
            </ActionIcon>
          )}
          <ActionIcon
            size={56}
            radius="xl"
            aria-label={t(state.paused ? 'remote.play' : 'remote.pause')}
            onClick={() => send({ type: state.paused ? 'PLAY' : 'PAUSE' })}
          >
            {state.paused ? <Play size={28} aria-hidden /> : <Pause size={28} aria-hidden />}
          </ActionIcon>
          {state.kind === 'RECORDING' && position !== null && (
            <ActionIcon
              variant="default"
              size="xl"
              radius="xl"
              aria-label={t('remote.forward', { seconds: skipSeconds })}
              onClick={() =>
                send({
                  type: 'SEEK',
                  position: Math.min(position + skipSeconds, state.duration ?? Infinity),
                })
              }
            >
              <FastForward size={22} aria-hidden />
            </ActionIcon>
          )}
        </Group>

        <Group gap="sm" wrap="nowrap">
          <ActionIcon
            variant="default"
            size="lg"
            aria-label={t(state.muted ? 'remote.unmute' : 'remote.mute')}
            aria-pressed={state.muted}
            onClick={() => send({ type: 'SET_MUTED', muted: !state.muted })}
          >
            {state.muted ? <VolumeX size={18} aria-hidden /> : <Volume2 size={18} aria-hidden />}
          </ActionIcon>
          <Slider
            flex={1}
            thumbLabel={t('remote.volume')}
            min={0}
            max={2}
            step={0.05}
            value={volume ?? state.volume}
            label={(value) => `${Math.round(value * 100)}%`}
            onChange={setVolume}
            onChangeEnd={(value) => send({ type: 'SET_VOLUME', volume: value })}
          />
        </Group>

        {state.audios.length >= 2 && (
          <SegmentedControl
            fullWidth
            aria-label={t('remote.audio')}
            value={state.audio ?? state.audios[0]?.id ?? ''}
            onChange={(id) => send({ type: 'SELECT_AUDIO', audio: id })}
            data={state.audios.map((audio) => ({ value: audio.id, label: audio.label }))}
          />
        )}

        {state.kind === 'LIVE' &&
          (state.dataBroadcast ? (
            <DataBroadcastRemote
              state="shown"
              receiving={false}
              keyGroups={keyGroups}
              onPress={(key) => {
                const name = remoteKeyName(key);
                if (name !== null) {
                  send({ type: 'PRESS_KEY', key: name });
                }
              }}
            />
          ) : (
            <Button
              variant="default"
              leftSection={<Tv size={16} aria-hidden />}
              onClick={() => send({ type: 'PRESS_KEY', key: remoteKeyName(RemoteKey.d) })}
            >
              {t('remote.showDataBroadcast')}
            </Button>
          ))}

        {/* 別のチャンネルや録画を開かせる (キャスト、#802)。 */}
        <Suspense fallback={<Loader size="sm" />}>
          <RemoteOpen onOpen={(target) => send({ type: 'OPEN', ...target })} />
        </Suspense>

        <Button
          variant="subtle"
          color="red"
          leftSection={<Square size={16} aria-hidden />}
          onClick={() => send({ type: 'STOP' })}
        >
          {t('remote.stop')}
        </Button>
      </Stack>
    </Card>
  );
}
