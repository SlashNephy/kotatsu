import {
  Alert,
  Anchor,
  Box,
  Button,
  Card,
  Group,
  Image,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Camera, Search } from 'lucide-react';
import { Suspense, useDeferredValue, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useMutation, useRelayEnvironment } from 'react-relay';

import {
  EmptyState,
  ErrorBoundary,
  PageLoader,
  ProgramTitle,
  toHalfWidth,
  useDocumentTitle,
  zonedFormat,
} from '../futon';
import { isUnauthenticated } from '../relay/environment';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import { useCaptureFileNameTemplate } from '../yunomi/captureFileNameTemplate';
import { captureFileName, copyBlob, saveBlob } from '../yunomi/capture';
import type {
  capturesListQuery,
  capturesListQuery$data,
} from './__generated__/capturesListQuery.graphql';
import type { capturesDeleteMutation } from './__generated__/capturesDeleteMutation.graphql';

export const Route = createFileRoute('/captures')({
  component: CapturesPage,
});

const timeFormat = zonedFormat({
  year: 'numeric',
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

// pageSize は、1 回に読み込むキャプチャの数。
const pageSize = 30;

const query = graphql`
  query capturesListQuery($first: Int!, $after: String, $keyword: String) {
    capturesAvailable
    captures(first: $first, after: $after, keyword: $keyword) {
      edges {
        node {
          id
          imageUrl
          serviceName
          programName
          recordingId
          position
          broadcastAt
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
`;

type Capture = capturesListQuery$data['captures']['edges'][number]['node'];

// CapturesPage は、サーバーに残したキャプチャの一覧 (#457、#908、設計書 7.5)。撮ったユーザーのものだけを出す。
// 番組名とサービス名で探し、保存、コピー、削除と、録画で撮ったものは撮った場面からの再生ができる。
function CapturesPage() {
  const { t } = useTranslation();
  useDocumentTitle([t('captures.title')]);
  const [keyword, setKeyword] = useState('');
  // 打っている間は、前の結果を出したまま探す。
  const deferred = useDeferredValue(keyword.trim());

  return (
    <Stack gap="sm">
      <Title order={2}>{t('captures.title')}</Title>
      <Text size="sm" c="dimmed">
        {t('captures.description')}
      </Text>
      <TextInput
        leftSection={<Search size={16} aria-hidden />}
        aria-label={t('captures.search')}
        placeholder={t('captures.search')}
        value={keyword}
        onChange={(event) => setKeyword(event.currentTarget.value)}
        maw={400}
      />
      <ErrorBoundary
        fallback={(error) =>
          isUnauthenticated(error) ? (
            <Alert color="amber">
              <Anchor component={Link} to="/login">
                {t('guide.signedOut')}
              </Anchor>
            </Alert>
          ) : (
            <Alert color="amber">{t('captures.loadError')}</Alert>
          )
        }
      >
        <Suspense fallback={<PageLoader />}>
          <CaptureList keyword={deferred} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

function CaptureList({ keyword }: { keyword: string }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  // deletions は、消した回数。消したら、最初のページから読み直す。
  const [deletions, setDeletions] = useState(0);
  const variables = { first: pageSize, after: null, keyword: keyword === '' ? null : keyword };
  const data = useLazyLoadQuery<capturesListQuery>(query, variables, {
    fetchKey: deletions,
    fetchPolicy: 'store-and-network',
  });
  // more は、続きの読み込みで足したキャプチャと、その次のカーソル。key は、読み込んだときの探す言葉と消した回数。
  // 探す言葉を変えるか消して、最初のページを読み直したら、続きは捨てる。
  const key = `${deletions}:${keyword}`;
  const [loaded, setMore] = useState<{
    key: string;
    captures: Capture[];
    next: string | null;
  } | null>(null);
  const more = loaded?.key === key ? loaded : null;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  // removed は、消したキャプチャ。読み直すまでの間も、一覧から外す。
  const [removed, setRemoved] = useState<ReadonlySet<string>>(new Set());
  const captures = [
    ...data.captures.edges.map((edge) => edge.node),
    ...(more?.captures ?? []),
  ].filter((capture) => !removed.has(capture.id));
  const next =
    more !== null
      ? more.next
      : data.captures.pageInfo.hasNextPage
        ? (data.captures.pageInfo.endCursor ?? null)
        : null;

  const loadMore = async () => {
    setLoading(true);
    setError(false);
    try {
      const page = await fetchQuery<capturesListQuery>(environment, query, {
        ...variables,
        after: next,
      }).toPromise();
      if (page !== undefined) {
        setMore({
          key,
          captures: [...(more?.captures ?? []), ...page.captures.edges.map((edge) => edge.node)],
          next: page.captures.pageInfo.hasNextPage
            ? (page.captures.pageInfo.endCursor ?? null)
            : null,
        });
      }
    } catch {
      setError(true);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Stack gap="sm">
      {!data.capturesAvailable && <Alert color="amber">{t('captures.unavailable')}</Alert>}
      {captures.length === 0 &&
        (keyword === '' ? (
          <EmptyState icon={Camera} title={t('captures.empty')} hint={t('captures.emptyHint')} />
        ) : (
          <Text c="dimmed">{t('captures.noMatch')}</Text>
        ))}
      <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }}>
        {captures.map((capture) => (
          <CaptureCard
            key={capture.id}
            capture={capture}
            onDeleted={() => {
              setRemoved((current) => new Set(current).add(capture.id));
              setDeletions((n) => n + 1);
            }}
          />
        ))}
      </SimpleGrid>
      {error && (
        <Alert color="red" role="alert">
          {t('captures.loadError')}
        </Alert>
      )}
      {next !== null && (
        <Box>
          <Button variant="default" loading={loading} onClick={() => void loadMore()}>
            {t('captures.more')}
          </Button>
        </Box>
      )}
    </Stack>
  );
}

// fetchImage は、キャプチャの画像を読む。サーバーに残した画像には、撮ったときに番組情報を書き込んである (#458)。
async function fetchImage(url: string): Promise<Blob> {
  const response = await fetch(url, { credentials: 'same-origin' });
  if (!response.ok) {
    throw new Error(`failed to fetch the capture: ${response.status}`);
  }

  return response.blob();
}

function CaptureCard({ capture, onDeleted }: { capture: Capture; onDeleted: () => void }) {
  const { t } = useTranslation();
  const [template] = useCaptureFileNameTemplate();
  const [message, setMessage] = useState<string | null>(null);
  const [remove, deleting] = useMutation<capturesDeleteMutation>(graphql`
    mutation capturesDeleteMutation($id: ID!) {
      deleteCapture(id: $id)
    }
  `);
  const time = Date.parse(capture.broadcastAt);

  const save = async () => {
    try {
      saveBlob(
        await fetchImage(capture.imageUrl),
        captureFileName(time, capture.serviceName, capture.programName ?? null, template),
      );
    } catch {
      setMessage(t('captures.actionError'));
    }
  };
  const copy = async () => {
    try {
      await copyBlob(await fetchImage(capture.imageUrl));
      setMessage(t('player.copied'));
    } catch {
      setMessage(t('player.captureError'));
    }
  };
  const del = () =>
    remove({
      variables: { id: capture.id },
      onCompleted: (_, errors) => {
        if (errors != null && errors.length > 0) {
          setMessage(t('captures.deleteError'));

          return;
        }
        onDeleted();
      },
      onError: () => setMessage(t('captures.deleteError')),
    });

  return (
    <Card withBorder padding="sm">
      <Card.Section>
        <Image
          src={capture.imageUrl}
          alt={toHalfWidth(capture.programName ?? capture.serviceName)}
          fit="contain"
          loading="lazy"
        />
      </Card.Section>
      <Stack gap={4} mt="xs">
        {capture.programName != null && <ProgramTitle name={capture.programName} />}
        <Text size="xs" c="dimmed">
          {toHalfWidth(capture.serviceName)} · {timeFormat.format(time)}
        </Text>
        <Group gap="xs" mt={4}>
          <Button size="compact-sm" variant="default" onClick={() => void save()}>
            {t('captures.save')}
          </Button>
          <Button size="compact-sm" variant="default" onClick={() => void copy()}>
            {t('captures.copy')}
          </Button>
          {capture.recordingId != null && (
            <RecordingLink recordingId={capture.recordingId} position={capture.position ?? null} />
          )}
          <Button size="compact-sm" variant="subtle" color="red" loading={deleting} onClick={del}>
            {t('captures.delete')}
          </Button>
        </Group>
        {message !== null && (
          <Text size="xs" c="dimmed" role="status">
            {message}
          </Text>
        )}
      </Stack>
    </Card>
  );
}

// RecordingLink は、キャプチャを撮った録画へのリンク。撮った位置が分かれば、その場面から再生する。分からなければ、録画の詳細を開く。
function RecordingLink({
  recordingId,
  position,
}: {
  recordingId: string;
  position: number | null;
}) {
  const { t } = useTranslation();

  return (
    <Anchor
      size="sm"
      renderRoot={(props) =>
        position === null ? (
          <Link {...props} to="/recordings/$recordingId" params={{ recordingId }} />
        ) : (
          <Link
            {...props}
            to="/recordings/$recordingId/play"
            params={{ recordingId }}
            search={{ t: Math.floor(position) }}
          />
        )
      }
    >
      {position === null ? t('captures.openRecording') : t('captures.playFrom')}
    </Anchor>
  );
}
