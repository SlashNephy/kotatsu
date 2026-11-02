import {
  Alert,
  Anchor,
  AspectRatio,
  Badge,
  Button,
  Card,
  Grid,
  Group,
  Modal,
  Stack,
  Switch,
  TagsInput,
  Text,
  Title,
} from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import {
  Download,
  Files,
  Gauge,
  Lock,
  LockOpen,
  Play,
  RefreshCw,
  ScanSearch,
  Save,
  Scissors,
  Tag,
  Trash2,
  type LucideIcon,
} from 'lucide-react';
import { Suspense, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';

import {
  ErrorBoundary,
  formatBytes,
  LinkifiedText,
  PageLoader,
  ProgramTitle,
  toHalfWidth,
} from '../../futon';
import { RecordingThumbnail } from '../../recordings/RecordingThumbnail';
import { AnimeEpisodes } from '../../anime/AnimeEpisodes';
import { RecordingStateBadge } from '../../recordings/RecordingStateBadge';
import { StreamQualityButton } from '../../recordings/StreamQualityButton';
import { recordingPosition } from '../../yunomi/position';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { RecordingIdDeleteMutation } from './__generated__/RecordingIdDeleteMutation.graphql';
import type { RecordingIdDetectCmMutation } from './__generated__/RecordingIdDetectCmMutation.graphql';
import type { RecordingIdProtectMutation } from './__generated__/RecordingIdProtectMutation.graphql';
import type { RecordingIdQuery } from './__generated__/RecordingIdQuery.graphql';
import type { RecordingIdReanalyzeMutation } from './__generated__/RecordingIdReanalyzeMutation.graphql';
import type { RecordingIdRegenerateMutation } from './__generated__/RecordingIdRegenerateMutation.graphql';
import type { RecordingIdTagsMutation } from './__generated__/RecordingIdTagsMutation.graphql';
import { ExternalStreamURL } from '../../streams/ExternalStreamURL';
import { programGenreLabels } from '../../guide/genreOptions';
import { ServiceLabel } from '../../guide/ServiceLabel';
import { zonedFormat } from '../../futon';
import { useDataChanges } from '../../changes/DataChanges';
import type { ChangeChannel } from '../../changes/dataChanges';
import { failureKey } from '../../recordings/failureReason';
import { SaveOfflineButton } from '../../offline/SaveOfflineButton';

export const Route = createFileRoute('/recordings/$recordingId')({
  component: RecordingPage,
});

// RecordingNotFoundError は、URL の録画が見つからないことを表す。通信の失敗とは分けて示す。
class RecordingNotFoundError extends Error {
  constructor() {
    super('recording not found');
    this.name = 'RecordingNotFoundError';
  }
}

const dateTimeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
const minuteFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});
const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

// RecordingPage は、録画の詳細の画面 (設計書 10.3)。保護、タグ付け、削除と、外部プレイヤーでの再生ができる。
function RecordingPage() {
  const { t } = useTranslation();
  const { recordingId } = Route.useParams();

  return (
    <Stack gap="sm">
      <Anchor component={Link} to="/recordings" size="sm">
        {t('recordings.back')}
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
              {error instanceof RecordingNotFoundError ||
              graphQLErrorDetails(error).code === 'NOT_FOUND'
                ? t('recordings.notFound')
                : t('viewer.error')}
            </Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <Recording key={recordingId} recordingId={recordingId} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// recordingChannels は、録画の詳細に関わるデータの種類。
const recordingChannels: readonly ChangeChannel[] = ['RECORDINGS'];

function Recording({ recordingId }: { recordingId: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  // 録画を終えた、サムネイルができた、ほかの端末で保護やタグを変えたなど、録画が変わったら読み直す (#868)。
  const changes = useDataChanges(recordingChannels);
  const data = useLazyLoadQuery<RecordingIdQuery>(
    graphql`
      query RecordingIdQuery($id: ID!) {
        viewer {
          role
        }
        tags {
          name
        }
        node(id: $id) {
          __typename
          ... on Recording {
            id
            name
            description
            startAt
            endAt
            state
            failureReason
            protected
            drops
            errors
            scrambled
            streamQuality {
              pid
              kind
              packets
              drops
              errors
              scrambled
              streamType
              componentTag
            }
            gaps {
              from
              to
            }
            programLost
            broadcastStartedAt
            broadcastEndedAt
            notBroadcast
            genres {
              lv1
              lv2
            }
            # 録画を始めたときに写した、しょぼいカレンダーで結びつけた話 (設計書 3.8)。
            animeEpisodes {
              syobocalUrl
              countStart
              countEnd
              subtitle
              first
              last
              rerun
              works {
                annictUrl
                anilistUrl
                malUrl
              }
            }
            rule {
              id
            }
            ruleName
            thumbnailUrl
            streamUrl
            videoFormat
            jikkyoCommentsUrl
            recordedFrom
            recordedUntil
            service {
              name
              logoUrl
              jikkyo {
                id
              }
            }
            tags {
              id
              name
            }
            files {
              kind
              path
              size
              url
            }
          }
        }
      }
    `,
    { id: recordingId },
    { fetchKey: changes, fetchPolicy: 'store-and-network' },
  );
  const [protect, protecting] = useMutation<RecordingIdProtectMutation>(graphql`
    mutation RecordingIdProtectMutation($id: ID!, $protected: Boolean!) {
      setRecordingProtected(id: $id, protected: $protected) {
        id
        protected
      }
    }
  `);
  const [saveTags, savingTags] = useMutation<RecordingIdTagsMutation>(graphql`
    mutation RecordingIdTagsMutation($id: ID!, $tags: [String!]!) {
      setRecordingTags(id: $id, tags: $tags) {
        id
        tags {
          id
          name
        }
      }
    }
  `);
  const [remove, removing] = useMutation<RecordingIdDeleteMutation>(graphql`
    mutation RecordingIdDeleteMutation($id: ID!) {
      deleteRecording(id: $id)
    }
  `);
  const [regenerate, regenerating] = useMutation<RecordingIdRegenerateMutation>(graphql`
    mutation RecordingIdRegenerateMutation($id: ID!) {
      regenerateRecordingImages(id: $id)
    }
  `);
  const [reanalyze, reanalyzing] = useMutation<RecordingIdReanalyzeMutation>(graphql`
    mutation RecordingIdReanalyzeMutation($id: ID!) {
      reanalyzeRecording(id: $id)
    }
  `);
  const [detectCm, detectingCm] = useMutation<RecordingIdDetectCmMutation>(graphql`
    mutation RecordingIdDetectCmMutation($id: ID!) {
      detectRecordingCm(id: $id)
    }
  `);
  // regenerated は、画像の作り直しか解析のやり直しか CM の解析を頼んだ結果。queued は入れた、waiting は前に頼んだものを待っている。
  const [regenerated, setRegenerated] = useState<{
    kind: 'images' | 'analysis' | 'cm';
    result: 'queued' | 'waiting';
  } | null>(null);
  const node = data.node;
  // editedTags は、編集中のタグ。編集していなければ null で、サーバーのタグをそのまま出す。
  const [editedTags, setEditedTags] = useState<string[] | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (node?.__typename !== 'Recording') {
    throw new RecordingNotFoundError();
  }
  const recording = node;
  const canEdit = data.viewer.role !== 'VIEWER';
  const savedTags = recording.tags.map((tag) => tag.name);
  const tags = editedTags ?? savedTags;
  const tagsChanged = JSON.stringify(tags) !== JSON.stringify(savedTags);
  const segments = recording.files.filter((file) => file.kind === 'SEGMENT');
  const size = segments.reduce((sum, file) => sum + Number(file.size), 0);
  // others は、EPGStation から取り込んだ、エンコード済みの MP4 のような TS 以外のファイル (設計書 6.3)。
  const others = recording.files.filter((file) => file.kind === 'OTHER' && file.url != null);
  const failed = (errors: readonly unknown[] | null | undefined) =>
    errors != null && errors.length > 0;

  return (
    <Grid>
      <Grid.Col span={{ base: 12, md: 5 }}>
        <Stack>
          <Card withBorder padding={0}>
            <AspectRatio ratio={16 / 9}>
              <RecordingThumbnail url={recording.thumbnailUrl} />
            </AspectRatio>
          </Card>
          {/* 画像の不具合を直したときなどに、代表画像とプレビュー画像を作り直す (#476)。作り直すまでは、今の画像を出す。 */}
          {canEdit && segments.length > 0 && recording.state !== 'RECORDING' && (
            <Group gap="xs">
              <Button
                size="xs"
                variant="subtle"
                leftSection={<RefreshCw size={14} aria-hidden />}
                loading={regenerating}
                onClick={() =>
                  regenerate({
                    variables: { id: recording.id },
                    onCompleted: (response, errors) => {
                      if (failed(errors)) {
                        setError(t('recordings.actionError'));

                        return;
                      }
                      setRegenerated({
                        kind: 'images',
                        result: response.regenerateRecordingImages ? 'queued' : 'waiting',
                      });
                    },
                    onError: () => setError(t('recordings.actionError')),
                  })
                }
              >
                {t('recordings.regenerateImages')}
              </Button>
              {/* 索引とドロップなどの集計を、録画のファイルから作り直す。終われば画像も作り直す (#476)。 */}
              <Button
                size="xs"
                variant="subtle"
                leftSection={<ScanSearch size={14} aria-hidden />}
                loading={reanalyzing}
                onClick={() =>
                  reanalyze({
                    variables: { id: recording.id },
                    onCompleted: (response, errors) => {
                      if (failed(errors)) {
                        setError(t('recordings.actionError'));

                        return;
                      }
                      setRegenerated({
                        kind: 'analysis',
                        result: response.reanalyzeRecording ? 'queued' : 'waiting',
                      });
                    },
                    onError: () => setError(t('recordings.actionError')),
                  })
                }
              >
                {t('recordings.reanalyze')}
              </Button>
              {/* 全体や予約の設定にかかわらず、CM の区間を見分ける。前の区間は置き換える (#1209)。 */}
              <Button
                size="xs"
                variant="subtle"
                leftSection={<Scissors size={14} aria-hidden />}
                loading={detectingCm}
                onClick={() =>
                  detectCm({
                    variables: { id: recording.id },
                    onCompleted: (response, errors) => {
                      if (failed(errors)) {
                        setError(t('recordings.actionError'));

                        return;
                      }
                      setRegenerated({
                        kind: 'cm',
                        result: response.detectRecordingCm ? 'queued' : 'waiting',
                      });
                    },
                    onError: () => setError(t('recordings.actionError')),
                  })
                }
              >
                {t('recordings.detectCm')}
              </Button>
              {regenerated !== null && (
                <Text size="xs" c="dimmed" role="status">
                  {t(
                    regenerated.kind === 'images'
                      ? regenerated.result === 'queued'
                        ? 'recordings.regenerateQueued'
                        : 'recordings.regenerateWaiting'
                      : regenerated.kind === 'analysis'
                        ? regenerated.result === 'queued'
                          ? 'recordings.reanalyzeQueued'
                          : 'recordings.reanalyzeWaiting'
                        : regenerated.result === 'queued'
                          ? 'recordings.detectCmQueued'
                          : 'recordings.detectCmWaiting',
                  )}
                </Text>
              )}
            </Group>
          )}
          {segments.length > 0 && (
            <Button
              leftSection={<Play size={16} fill="currentColor" aria-hidden />}
              renderRoot={(props) => (
                <Link {...props} to="/recordings/$recordingId/play" params={{ recordingId }} />
              )}
            >
              {t('recordings.play')}
            </Button>
          )}
          {/* 外部プレイヤー用の URL は、TS のセグメントを流す。セグメントのない取り込んだ録画では出さない。 */}
          {segments.length > 0 && <ExternalStreamURL target={recording.id} />}
          {/* 録画を終えた録画を、セグメントをつないだ 1 つのファイルとして保存させる。録画中は長さが決まらないので出さない。 */}
          {segments.length > 0 && recording.state !== 'RECORDING' && (
            <Button
              variant="default"
              leftSection={<Download size={16} aria-hidden />}
              component="a"
              href={`${recording.streamUrl}?download=1`}
              download
            >
              {t('recordings.download')}
            </Button>
          )}
          {/* 録画を終えた録画を端末に保存し、サーバーにつながらなくても見られるようにする (#480)。 */}
          {segments.length > 0 &&
            recording.state !== 'RECORDING' &&
            recording.recordedUntil != null && (
              <SaveOfflineButton
                videoFormat={recording.videoFormat}
                source={{
                  id: recording.id,
                  name: recording.name,
                  serviceName: recording.service.name,
                  startAt: recording.startAt,
                  endAt: recording.endAt,
                  recordedFrom: recording.recordedFrom,
                  recordedUntil: recording.recordedUntil,
                  gaps: recording.gaps,
                  streamUrl: recording.streamUrl,
                  // 実況のチャンネルのない局では、過去ログを読まない (設計書 7.4)。
                  jikkyoCommentsUrl:
                    recording.service.jikkyo == null ? null : recording.jikkyoCommentsUrl,
                  files: recording.files,
                }}
              />
            )}
        </Stack>
      </Grid.Col>
      <Grid.Col span={{ base: 12, md: 7 }}>
        <Stack>
          <Title order={2} size="h3">
            <ProgramTitle name={recording.name} />
          </Title>
          <Group gap="xs">
            <Text size="sm">
              <ServiceLabel name={recording.service.name} logoUrl={recording.service.logoUrl} />
            </Text>
            <Text size="sm" c="dimmed">
              {t('range', {
                from: dateTimeFormat.format(Date.parse(recording.startAt)),
                to: minuteFormat.format(Date.parse(recording.endAt)),
              })}
            </Text>
            {/* 完了は当たり前なので出さず、録画中や失敗のように目を留めたい状態だけを出す。 */}
            {recording.state !== 'COMPLETED' && <RecordingStateBadge state={recording.state} />}
            {recording.protected && (
              <Badge color="gray" variant="outline" leftSection={<Lock size={12} aria-hidden />}>
                {t('recordings.protected')}
              </Badge>
            )}
            {/* ジャンルは、番組の詳細と同じく「大分類／中分類」ですべて出す。 */}
            {programGenreLabels(t, recording.genres).map((genre) => (
              <Badge key={genre} variant="light">
                {genre}
              </Badge>
            ))}
          </Group>
          <AnimeEpisodes episodes={recording.animeEpisodes} />
          {/* ルールで録った録画は、そのルールへ移れるようにする。ルールを消していても、名前は出す (#468)。 */}
          {recording.ruleName != null &&
            (recording.rule != null ? (
              <Anchor
                size="sm"
                renderRoot={(props) => (
                  <Link
                    {...props}
                    to="/rules/$ruleId"
                    params={{ ruleId: recording.rule?.id ?? '' }}
                  />
                )}
              >
                {t('recordings.recordedByRule', { name: recording.ruleName })}
              </Anchor>
            ) : (
              <Text size="sm" c="dimmed">
                {t('recordings.recordedByDeletedRule', { name: recording.ruleName })}
              </Text>
            ))}
          {recording.failureReason != null && (
            <Alert color="red">{t(failureKey(recording.failureReason))}</Alert>
          )}
          {recording.description !== '' && (
            <Text size="sm">
              <LinkifiedText text={toHalfWidth(recording.description)} />
            </Text>
          )}
          <Card withBorder>
            <Stack gap={4}>
              <CardHeading icon={Gauge}>{t('recordings.quality')}</CardHeading>
              <Text size="sm">
                {t('recordings.qualityCounts', {
                  drops: recording.drops,
                  errors: recording.errors,
                  scrambled: recording.scrambled,
                })}
              </Text>
              <StreamQualityButton
                streams={recording.streamQuality}
                // 終えた日時がなく、録画中でもない録画は、長さが分からないので、ビットレートを出さない。
                seconds={
                  recording.recordedUntil != null
                    ? recordingPosition(
                        recording.recordedFrom,
                        recording.gaps,
                        Date.parse(recording.recordedUntil),
                      )
                    : recording.state === 'RECORDING'
                      ? recordingPosition(recording.recordedFrom, recording.gaps, Date.now())
                      : 0
                }
              />
              <Text size="sm" c="dimmed">
                {t('recordings.recordedRange', {
                  from: timeFormat.format(Date.parse(recording.recordedFrom)),
                  // 終えた日時がないのは、録画中か、終えた日時を記録できずに止まった録画。
                  until:
                    recording.recordedUntil != null
                      ? timeFormat.format(Date.parse(recording.recordedUntil))
                      : recording.state === 'RECORDING'
                        ? t('recordings.state.RECORDING')
                        : t('recordings.unknownEnd'),
                })}
                {segments.length > 0 &&
                  ` ・ ${t('recordings.size', { size: formatBytes(size), count: segments.length })}`}
              </Text>
              {recording.gaps.map((gap, index) => (
                <Text key={index} size="sm" c="amber.8">
                  {t('recordings.gap', {
                    from: timeFormat.format(Date.parse(gap.from)),
                    to: timeFormat.format(Date.parse(gap.to)),
                  })}
                </Text>
              ))}
              {recording.programLost && (
                <Text size="sm" c="amber.8">
                  {t('recordings.programLost')}
                </Text>
              )}
              {/* 録画中の TS の EIT[p/f] で分かった、実際の放送の時刻 (#389)。 */}
              {recording.broadcastStartedAt != null && (
                <Text size="sm" c="dimmed">
                  {t('recordings.broadcastRange', {
                    from: timeFormat.format(Date.parse(recording.broadcastStartedAt)),
                    until:
                      recording.broadcastEndedAt != null
                        ? timeFormat.format(Date.parse(recording.broadcastEndedAt))
                        : t('recordings.broadcastOngoing'),
                  })}
                </Text>
              )}
              {recording.notBroadcast && (
                <Text size="sm" c="amber.8">
                  {t('recordings.notBroadcast')}
                </Text>
              )}
            </Stack>
          </Card>
          {others.length > 0 && (
            <Card withBorder>
              <Stack gap="xs">
                <CardHeading icon={Files}>{t('recordings.otherFiles')}</CardHeading>
                <Text size="xs" c="dimmed">
                  {t('recordings.otherFilesDescription')}
                </Text>
                {others.map((file) => (
                  <Stack key={file.url} gap={4}>
                    <Text size="sm">
                      {file.path.split('/').pop()} ・ {formatBytes(Number(file.size))}
                    </Text>
                    {/* ブラウザがそのコーデックに対応していれば、そのまま再生できる。 */}
                    <video
                      controls
                      preload="metadata"
                      src={file.url ?? undefined}
                      style={{ width: '100%' }}
                    >
                      <track kind="captions" />
                    </video>
                  </Stack>
                ))}
              </Stack>
            </Card>
          )}
          <Card withBorder>
            <Stack gap="xs">
              <TagsInput
                label={<CardHeading icon={Tag}>{t('recordings.tags')}</CardHeading>}
                data={data.tags.map((tag) => tag.name)}
                value={tags}
                onChange={setEditedTags}
                disabled={!canEdit}
                clearable
              />
              {canEdit && (
                <Group>
                  <Button
                    size="xs"
                    variant="default"
                    leftSection={<Save size={14} aria-hidden />}
                    disabled={!tagsChanged}
                    loading={savingTags}
                    onClick={() =>
                      saveTags({
                        variables: { id: recording.id, tags },
                        onCompleted: (_response, errors) => {
                          if (failed(errors)) {
                            setError(t('recordings.actionError'));

                            return;
                          }
                          // 保存したら、サーバーが返したタグを出す。
                          setError(null);
                          setEditedTags(null);
                        },
                        onError: () => setError(t('recordings.actionError')),
                      })
                    }
                  >
                    {t('recordings.saveTags')}
                  </Button>
                </Group>
              )}
            </Stack>
          </Card>
          {canEdit && (
            <Group justify="space-between">
              <Switch
                label={t('recordings.protect')}
                thumbIcon={
                  recording.protected ? (
                    <Lock size={10} color="var(--mantine-primary-color-filled)" aria-hidden />
                  ) : (
                    <LockOpen size={10} color="var(--mantine-color-gray-6)" aria-hidden />
                  )
                }
                description={t('recordings.protectDescription')}
                checked={recording.protected}
                disabled={protecting}
                onChange={(e) =>
                  protect({
                    variables: { id: recording.id, protected: e.currentTarget.checked },
                    onCompleted: (_response, errors) =>
                      setError(failed(errors) ? t('recordings.actionError') : null),
                    onError: () => setError(t('recordings.actionError')),
                  })
                }
              />
              <Button
                color="red"
                variant="subtle"
                leftSection={<Trash2 size={16} aria-hidden />}
                // 保護した録画と、録画中の録画は消せない。
                disabled={recording.protected || recording.state === 'RECORDING'}
                onClick={() => setDeleting(true)}
              >
                {t('recordings.delete')}
              </Button>
            </Group>
          )}
          {error !== null && (
            <Alert color="red" role="alert">
              {error}
            </Alert>
          )}
        </Stack>
      </Grid.Col>
      {deleting && (
        <Modal opened onClose={() => setDeleting(false)} title={t('recordings.deleteTitle')}>
          <Stack>
            <Text size="sm" fw={700}>
              <ProgramTitle name={recording.name} />
            </Text>
            <Text size="sm">{t('recordings.deleteDescription')}</Text>
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setDeleting(false)}>
                {t('recordings.cancel')}
              </Button>
              <Button
                color="red"
                leftSection={<Trash2 size={16} aria-hidden />}
                loading={removing}
                onClick={() =>
                  remove({
                    variables: { id: recording.id },
                    onCompleted: (_response, errors) => {
                      if (failed(errors)) {
                        setDeleting(false);
                        setError(t('recordings.deleteError'));

                        return;
                      }
                      void navigate({ to: '/recordings' });
                    },
                    onError: () => {
                      setDeleting(false);
                      setError(t('recordings.deleteError'));
                    },
                  })
                }
              >
                {t('recordings.delete')}
              </Button>
            </Group>
          </Stack>
        </Modal>
      )}
    </Grid>
  );
}

// CardHeading は、カードの見出し。何のカードか目で追えるよう、アイコンを添える。
function CardHeading({ icon: Icon, children }: { icon: LucideIcon; children: ReactNode }) {
  return (
    <Group gap={6} component="span" wrap="nowrap">
      <Icon size={16} aria-hidden />
      <Text component="span" size="sm" fw={700}>
        {children}
      </Text>
    </Group>
  );
}
