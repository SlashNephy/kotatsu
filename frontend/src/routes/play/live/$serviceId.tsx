import { Alert, Anchor, Stack } from '@mantine/core';
import { createFileRoute, Link } from '@tanstack/react-router';
import { startTransition, Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';
import { useLazyLoadQuery } from '../../../relay/useLazyLoadQuery';

import { ErrorBoundary, PageLoader } from '../../../futon';
import { graphQLErrorDetails, isUnauthenticated } from '../../../relay/environment';
import type { ServiceIdLiveQuery } from './__generated__/ServiceIdLiveQuery.graphql';
import type { ServiceIdRecordingsQuery } from './__generated__/ServiceIdRecordingsQuery.graphql';
import { useHostedWatch, useWatchMini } from '../../../yunomi/WatchHost';
import { readableParams } from '../../../futon/readableId';
import { LivePlayer } from '../../../yunomi/LivePlayer';

export const Route = createFileRoute('/play/live/$serviceId')({
  // URL には素の ID を出し、画面には GraphQL の ID を渡す (#1346)。
  params: readableParams('serviceId', 'Service'),
  // 右のカラムを置くので、本文の幅の上限を外し、WatchLayout で映像が画面の高さに収まる幅までにする。
  staticData: { fluid: true },
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
// 画面は、アプリのルート要素に描く (#1602)。
function LiveServicePage() {
  const { serviceId } = Route.useParams();

  return useHostedWatch(<LiveServiceWatch serviceId={serviceId} />);
}

// LiveServiceWatch は、サービス serviceId のライブ視聴の画面の内容。
function LiveServiceWatch({ serviceId }: { serviceId: string }) {
  const { t } = useTranslation();
  // 小さなプレイヤー (#1602) では、戻るリンクを出さず、映像だけにする。
  const mini = useWatchMini();
  // 放送中の番組を出すので、画面を開いた時刻を、待たない親で決める。
  const [range, setRange] = useState(() => liveRange(Date.now()));
  // 番組が終わったら、次の番組を読み直す。読み直す間も、再生は続ける。
  const refresh = () => startTransition(() => setRange(liveRange(Date.now())));

  return (
    <Stack gap="sm">
      {!mini && (
        <Anchor component={Link} to="/live" size="sm">
          {t('live.back')}
        </Anchor>
      )}
      {/* 同じルートのまま別のサービスへ移ったときは、前のサービスのエラーを残さず読み直す (#1453)。 */}
      <ErrorBoundary
        resetKey={serviceId}
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

// liveRange は、now から、次の番組まで読める時間帯。放送中の番組と、その次の番組を取得するのに使う (#438)。
function liveRange(now: number): { now: number; from: string; to: string } {
  return {
    now,
    from: new Date(now).toISOString(),
    to: new Date(now + liveLookahead).toISOString(),
  };
}

// liveLookahead は、次の番組を探す、いまからの時間。
const liveLookahead = 6 * 3_600_000;

// Live は、サービスと放送中の番組を読み、ライブ視聴のプレイヤーに渡す。
function Live({
  serviceId,
  range,
  onProgramEnd,
}: {
  serviceId: string;
  range: { now: number; from: string; to: string };
  onProgramEnd: () => void;
}) {
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
            videoFormat
            jikkyoChannelId
            jikkyo {
              id
              name
            }
            # タイムシフトのバッファにある番組の一覧を、メニューから開けるかを知る (設計書 5.7)。
            timeshiftPrograms {
              startAt
            }
            # タイムシフトのバッファの範囲。ライブ視聴から、過去へシークするのに使う (設計書 5.7)。
            timeshiftBuffer {
              from
              gaps {
                from
                to
              }
              streamUrl
              hlsUrl
            }
          }
        }
        guide(from: $from, to: $to, services: [$id]) {
          programs {
            id
            name
            description
            extended {
              heading
              text
            }
            startAt
            endAt
            endUndecided
            audios {
              componentType
              languages
              samplingRate
            }
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
        # データ放送の文書の双方向の通信を、tenban に中継させるか (#501)。
        systemSettings {
          dataBroadcastIpEnabled
          # 外部プレイヤー用の URL は、管理者が有効にしているときだけ発行できる (#1755)。
          externalPlayerUrlsEnabled
          # 視聴している間だけ録るなら、バッファができるのを待って読み直す (#1408)。
          timeshiftWhileWatching
        }
        # OS のメディア操作の前と次で移る局と、データ放送の選局で移る先を検索する、番組表と同じ並びのサービス (#493、#440)。
        services {
          id
          name
          networkId
          serviceId
          channelType {
            type
          }
        }
      }
    `,
    { id: serviceId, from: range.from, to: range.to },
  );

  return (
    <LivePlayer
      source={{
        serviceId,
        data,
        range,
        onProgramEnd,
        // 切れたときに、チューナーを使っている録画を、理由として示すために読む。
        fetchRecordings: () =>
          fetchQuery<ServiceIdRecordingsQuery>(
            environment,
            recordingsQuery,
            {},
            { fetchPolicy: 'network-only' },
          )
            .toPromise()
            .then(
              (result) =>
                result?.recordings.edges.map((edge) => ({
                  name: edge.node.name,
                  recordedFrom: edge.node.recordedFrom,
                  serviceName: edge.node.service.name,
                })) ?? [],
            ),
      }}
    />
  );
}
