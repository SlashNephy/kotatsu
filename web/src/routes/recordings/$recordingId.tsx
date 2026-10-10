import {
  Alert,
  Anchor,
  AspectRatio,
  Badge,
  Button,
  Card,
  CopyButton,
  Grid,
  Group,
  Modal,
  Stack,
  Switch,
  TagsInput,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { RecordingThumbnail } from '../../recordings/RecordingThumbnail';
import { stateColor, stateKey } from '../../recordings/states';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import type { RecordingIdDeleteMutation } from './__generated__/RecordingIdDeleteMutation.graphql';
import type { RecordingIdProtectMutation } from './__generated__/RecordingIdProtectMutation.graphql';
import type { RecordingIdQuery } from './__generated__/RecordingIdQuery.graphql';
import type { RecordingIdStreamURLMutation } from './__generated__/RecordingIdStreamURLMutation.graphql';
import type { RecordingIdTagsMutation } from './__generated__/RecordingIdTagsMutation.graphql';

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

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});
const minuteFormat = new Intl.DateTimeFormat('ja-JP', {
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});
const timeFormat = new Intl.DateTimeFormat('ja-JP', {
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

const failureKeys = {
  TUNER_UNAVAILABLE: 'recordings.failure.TUNER_UNAVAILABLE',
  TUNER_SERVER_UNREACHABLE: 'recordings.failure.TUNER_SERVER_UNREACHABLE',
  STREAM_INTERRUPTED: 'recordings.failure.STREAM_INTERRUPTED',
  STORAGE_UNAVAILABLE: 'recordings.failure.STORAGE_UNAVAILABLE',
  STORAGE_FULL: 'recordings.failure.STORAGE_FULL',
  PROGRAM_NOT_FOUND: 'recordings.failure.PROGRAM_NOT_FOUND',
  CANCELLED: 'recordings.failure.CANCELLED',
} as const;

// formatBytes は、バイト数を、GB か MB で表す。
function formatBytes(bytes: number): string {
  return bytes >= 1e9 ? `${(bytes / 1e9).toFixed(1)} GB` : `${(bytes / 1e6).toFixed(1)} MB`;
}

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
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Recording key={recordingId} recordingId={recordingId} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function Recording({ recordingId }: { recordingId: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
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
            gaps {
              from
              to
            }
            programLost
            thumbnailUrl
            recordedFrom
            recordedUntil
            service {
              name
            }
            tags {
              id
              name
            }
            files {
              kind
              size
            }
          }
        }
      }
    `,
    { id: recordingId },
    { fetchPolicy: 'store-and-network' },
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
  const [createURL, creatingURL] = useMutation<RecordingIdStreamURLMutation>(graphql`
    mutation RecordingIdStreamURLMutation($target: ID!) {
      createStreamURL(target: $target) {
        url
        expiresAt
      }
    }
  `);
  const node = data.node;
  // editedTags は、編集中のタグ。編集していなければ null で、サーバーのタグをそのまま出す。
  const [editedTags, setEditedTags] = useState<string[] | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [streamURL, setStreamURL] = useState<string | null>(null);
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
  const failed = (errors: readonly unknown[] | null | undefined) =>
    errors != null && errors.length > 0;

  const openExternally = () => {
    setError(null);
    createURL({
      variables: { target: recording.id },
      onCompleted: (response, errors) =>
        failed(errors)
          ? setError(t('recordings.actionError'))
          : setStreamURL(new URL(response.createStreamURL.url, window.location.origin).href),
      onError: () => setError(t('recordings.actionError')),
    });
  };

  return (
    <Grid>
      <Grid.Col span={{ base: 12, md: 5 }}>
        <Stack>
          <Card withBorder padding={0}>
            <AspectRatio ratio={16 / 9}>
              <RecordingThumbnail url={recording.thumbnailUrl} />
            </AspectRatio>
          </Card>
          <Button onClick={openExternally} loading={creatingURL}>
            {t('recordings.openExternally')}
          </Button>
          {streamURL !== null && (
            <Stack gap={4}>
              <Text size="xs" c="dimmed">
                {t('recordings.streamURLDescription')}
              </Text>
              <Group gap="xs" wrap="nowrap">
                <TextInput value={streamURL} readOnly style={{ flex: 1 }} size="xs" />
                <CopyButton value={streamURL}>
                  {({ copied, copy }) => (
                    <Button size="xs" variant="default" onClick={copy}>
                      {copied ? t('recordings.copied') : t('recordings.copy')}
                    </Button>
                  )}
                </CopyButton>
              </Group>
            </Stack>
          )}
        </Stack>
      </Grid.Col>
      <Grid.Col span={{ base: 12, md: 7 }}>
        <Stack>
          <Title order={2} size="h3">
            {recording.name}
          </Title>
          <Group gap="xs">
            <Text size="sm">{recording.service.name}</Text>
            <Text size="sm" c="dimmed">
              {dateTimeFormat.format(Date.parse(recording.startAt))}〜
              {minuteFormat.format(Date.parse(recording.endAt))}
            </Text>
            <Badge color={stateColor(recording.state)} variant="light">
              {t(stateKey(recording.state))}
            </Badge>
            {recording.protected && (
              <Badge color="gray" variant="outline">
                {t('recordings.protected')}
              </Badge>
            )}
          </Group>
          {recording.failureReason != null && (
            <Alert color="red">
              {t(
                failureKeys[recording.failureReason as keyof typeof failureKeys] ??
                  'recordings.failure.unknown',
              )}
            </Alert>
          )}
          {recording.description !== '' && <Text size="sm">{recording.description}</Text>}
          <Card withBorder>
            <Stack gap={4}>
              <Text size="sm" fw={700}>
                {t('recordings.quality')}
              </Text>
              <Text size="sm">
                {t('recordings.qualityCounts', {
                  drops: recording.drops,
                  errors: recording.errors,
                  scrambled: recording.scrambled,
                })}
              </Text>
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
            </Stack>
          </Card>
          <Card withBorder>
            <Stack gap="xs">
              <TagsInput
                label={t('recordings.tags')}
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
              {recording.name}
            </Text>
            <Text size="sm">{t('recordings.deleteDescription')}</Text>
            <Group justify="flex-end">
              <Button variant="default" onClick={() => setDeleting(false)}>
                {t('recordings.cancel')}
              </Button>
              <Button
                color="red"
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
