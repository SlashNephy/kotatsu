import { Alert, Anchor, Stack, Text, Title } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Suspense, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useLazyLoadQuery, useRelayEnvironment } from 'react-relay';

import { ErrorBoundary } from '../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../relay/environment';
import { reconnectDelay, recordingsThatTookTuner } from '../../yunomi/reconnect';
import { TsPlayer } from '../../yunomi/TsPlayer';
import { useVisible } from '../../yunomi/useVisible';
import type { ServiceIdLiveQuery } from './__generated__/ServiceIdLiveQuery.graphql';
import type { ServiceIdRecordingsQuery } from './__generated__/ServiceIdRecordingsQuery.graphql';

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
          }
        }
      }
    }
  }
`;

// LiveServicePage は、サービスをブラウザの中で変換してライブ視聴する画面 (設計書 7.2)。
function LiveServicePage() {
  const { t } = useTranslation();
  const { serviceId } = Route.useParams();
  // 放送中の番組を出すので、画面を開いた時刻を、待たない親で決める。
  const [range] = useState(() => {
    const now = Date.now();

    return { from: new Date(now).toISOString(), to: new Date(now + 60_000).toISOString() };
  });

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
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Live key={serviceId} serviceId={serviceId} range={range} />
        </Suspense>
      </ErrorBoundary>
    </Stack>
  );
}

// Disconnection は、視聴が切れたことと、つなぎ直すまでの秒と、切れた理由とみなす録画。
type Disconnection = {
  at: number;
  retryIn: number;
  recordings: { name: string; serviceName: string }[];
};

function Live({ serviceId, range }: { serviceId: string; range: { from: string; to: string } }) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const data = useLazyLoadQuery<ServiceIdLiveQuery>(
    graphql`
      query ServiceIdLiveQuery($id: ID!, $from: Time!, $to: Time!) {
        node(id: $id) {
          __typename
          ... on Service {
            name
            streamUrl
          }
        }
        guide(from: $from, to: $to, services: [$id]) {
          programs {
            name
          }
        }
      }
    `,
    { id: serviceId, ...range },
  );
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const visible = useVisible(videoRef);
  // session は、つなぎ直すたびに増やし、URL を変えて読み直させる。
  const [session, setSession] = useState(0);
  const attempt = useRef(0);
  const [disconnection, setDisconnection] = useState<Disconnection | null>(null);

  // 視聴が切れたら、理由を調べ、間隔を延ばしながらつなぎ直す。
  const disconnected = () => {
    if (disconnection !== null) {
      return;
    }
    const at = Date.now();
    const retried = attempt.current > 0;
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

  const service = data.node?.__typename === 'Service' ? data.node : null;
  if (service === null) {
    return <Alert color="amber">{t('live.notFound')}</Alert>;
  }
  // 画面の外に出たときと、タブが隠れたときは、読み込みも止めて、チューナーを手放す (設計書 7.2)。
  const url = `${service.streamUrl}?session=${session}`;

  return (
    <Stack gap="xs">
      <Title order={2} size="h4">
        {service.name}
      </Title>
      <Text size="sm" c="dimmed">
        {data.guide[0]?.programs[0]?.name ?? t('live.noProgram')}
      </Text>
      <TsPlayer
        url={disconnection === null ? url : null}
        videoRef={videoRef}
        active={visible}
        onLoaded={() => {
          attempt.current = 0;
        }}
        onState={(state) => {
          // ライブのストリームは終わらないので、読み終えたのは、切れたということ。
          if (state === 'completed' || state === 'error') {
            disconnected();
          }
        }}
        onError={disconnected}
      />
      {!visible && (
        <Text size="sm" c="dimmed">
          {t('live.paused')}
        </Text>
      )}
      {disconnection !== null && (
        <Alert color="amber" role="alert" title={t('live.disconnected')}>
          <Stack gap={4}>
            {disconnection.recordings.length > 0 ? (
              <Text size="sm">
                {t('live.tunerTaken', {
                  recordings: disconnection.recordings
                    .map((recording) => `${recording.serviceName}「${recording.name}」`)
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
    </Stack>
  );
}
