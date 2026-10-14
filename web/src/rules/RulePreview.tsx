import { Alert, Badge, Card, Group, Loader, Stack, Text, Title } from '@mantine/core';
import { SearchX } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchQuery, graphql, useRelayEnvironment } from 'react-relay';

import type {
  RulePreviewQuery,
  RulePreviewQuery$data,
  RulePreviewQuery$variables,
} from './__generated__/RulePreviewQuery.graphql';
import { ProgramList } from '../conditions/ProgramList';
import { EmptyState } from '../futon';
import { ProgramDetail } from '../guide/ProgramDetail';

type Props = {
  input: RulePreviewQuery$variables['input'];
  // enabled は、プレビューを取るかどうか。条件がなければ、すべての番組に一致してしまうので取らない。
  enabled: boolean;
  // onPrograms は、プレビューに出した番組を受け取る。ファイル名のテンプレートを当てはめる番組の候補に使う。
  onPrograms?: (programs: { id: string; name: string }[]) => void;
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

type Preview = RulePreviewQuery$data['previewRule'];

// RulePreview は、編集中のルールに一致するこれからの番組を、その場で示す (設計書 4.3)。
export function RulePreview({ input, enabled, onPrograms }: Props) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [preview, setPreview] = useState<Preview | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [selected, setSelected] = useState<string | undefined>(undefined);
  // 渡されるたびに作り直される関数で取り直さないよう、ref で持つ。
  const onProgramsRef = useRef(onPrograms);
  onProgramsRef.current = onPrograms;
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
              onProgramsRef.current?.(
                data.previewRule.edges.map(({ node }) => ({ id: node.id, name: node.name })),
              );
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
            <ProgramList programs={preview.edges.map(({ node }) => node)} onSelect={setSelected} />
            {preview.pageInfo.hasNextPage && (
              <Badge variant="light" color="gray">
                {t('rules.preview.more')}
              </Badge>
            )}
          </Stack>
        )}
      </Stack>
      <ProgramDetail programId={selected} onClose={() => setSelected(undefined)} />
    </Card>
  );
}
