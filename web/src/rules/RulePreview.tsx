import { Alert, Badge, Card, Group, Loader, Stack, Text, Title } from '@mantine/core';
import { SearchX } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';

import type {
  RulePreviewQuery,
  RulePreviewQuery$data,
  RulePreviewQuery$variables,
} from './__generated__/RulePreviewQuery.graphql';
import { EmptyState, ProgramTitle } from '../futon';
import { ServiceLabel } from '../guide/ServiceLabel';

type Props = {
  input: RulePreviewQuery$variables['input'];
  // enabled は、プレビューを取るかどうか。条件がなければ、すべての番組に一致してしまうので取らない。
  enabled: boolean;
};

const query = graphql`
  query RulePreviewQuery($input: RuleInput!) {
    previewRule(input: $input, first: 30) {
      edges {
        node {
          id
          name
          startAt
          service {
            name
            logoUrl
          }
        }
      }
      pageInfo {
        hasNextPage
      }
    }
  }
`;

// debounceMs は、入力が止まってからプレビューを取るまでの時間。打つたびに問い合わせないようにする。
const debounceMs = 400;

const dateTimeFormat = new Intl.DateTimeFormat('ja-JP', {
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
  timeZone: 'Asia/Tokyo',
});

type Preview = RulePreviewQuery$data['previewRule'];

// RulePreview は、編集中のルールに一致するこれからの番組を、その場で示す (設計書 4.3)。
export function RulePreview({ input, enabled }: Props) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [preview, setPreview] = useState<Preview | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  // 入力の中身が変わったときだけ取り直す。
  const key = JSON.stringify(input);

  useEffect(() => {
    if (!enabled) {
      // 条件を消したら、読み込み中とエラーの表示も消す。読み込み中の応答は、cleanup で捨てている。
      setPreview(null);
      setLoading(false);
      setError(false);

      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      setLoading(true);
      setError(false);
      // key から戻した入力で取る。effect は key の変化だけを見る。
      const variables = { input: JSON.parse(key) as Props['input'] };
      fetchQuery<RulePreviewQuery>(environment, query, variables, { fetchPolicy: 'network-only' })
        .toPromise()
        .then(
          (data) => {
            if (!cancelled && data !== undefined) {
              setPreview(data.previewRule);
            }
          },
          () => {
            if (!cancelled) {
              setError(true);
            }
          },
        )
        .finally(() => {
          if (!cancelled) {
            setLoading(false);
          }
        });
    }, debounceMs);

    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [key, enabled, environment]);

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Group justify="space-between">
          <Title order={3} size="h5">
            {t('rules.preview.title')}
          </Title>
          {loading && <Loader size="xs" />}
        </Group>
        {!enabled && (
          <Text size="sm" c="dimmed">
            {t('rules.preview.noConditions')}
          </Text>
        )}
        {error && (
          <Alert color="amber" role="alert">
            {t('rules.preview.error')}
          </Alert>
        )}
        {enabled && preview !== null && preview.edges.length === 0 && (
          <EmptyState
            icon={SearchX}
            title={t('rules.preview.empty')}
            hint={t('rules.preview.emptyHint')}
          />
        )}
        {enabled && preview !== null && preview.edges.length > 0 && (
          <Stack gap={2} data-testid="rule-preview">
            {preview.edges.map(({ node }) => (
              <Group key={node.id} gap="xs" wrap="nowrap">
                <Text size="xs" c="dimmed" w={120} style={{ flexShrink: 0 }}>
                  {dateTimeFormat.format(Date.parse(node.startAt))}
                </Text>
                <Text size="xs" w={110} truncate style={{ flexShrink: 0 }}>
                  <ServiceLabel name={node.service.name} logoUrl={node.service.logoUrl} />
                </Text>
                <Text size="xs" truncate>
                  <ProgramTitle name={node.name} />
                </Text>
              </Group>
            ))}
            {preview.pageInfo.hasNextPage && (
              <Badge variant="light" color="gray">
                {t('rules.preview.more')}
              </Badge>
            )}
          </Stack>
        )}
      </Stack>
    </Card>
  );
}
