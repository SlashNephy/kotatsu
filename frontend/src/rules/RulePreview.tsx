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
import { useDataChanges } from '../changes/DataChanges';
import type { ChangeChannel } from '../changes/dataChanges';
import { ProgramList } from '../conditions/ProgramList';
import { EmptyState, splitProgramMarks, toHalfWidth } from '../futon';
import { ProgramDetail } from '../guide/ProgramDetail';
import { zonedFormat } from '../futon';

type Props = {
  input: RulePreviewQuery$variables['input'];
  // enabled は、プレビューを取るかどうか。条件がなければ、すべての番組に一致してしまうので取らない。
  enabled: boolean;
  // onPrograms は、プレビューに出した番組を受け取る。ファイル名のテンプレートを当てはめる番組の候補に使う。
  onPrograms?: (programs: { id: string; name: string }[]) => void;
};

const query = graphql`
  query RulePreviewQuery($input: RuleInput!) {
    # 照合を重くする書き方 (#904)。警告にとどめ、保存は止めない。
    ruleWarnings(input: $input) {
      kind
      exclude
      index
    }
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

const programTime = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// previewChannels は、プレビューに関わるデータの種類。
const previewChannels: readonly ChangeChannel[] = ['EPG'];

// debounceMs は、入力が止まってからプレビューを取るまでの時間。打つたびに問い合わせないようにする。
const debounceMs = 400;

type Preview = RulePreviewQuery$data['previewRule'];
type Warning = RulePreviewQuery$data['ruleWarnings'][number];

// RulePreview は、編集中のルールに一致するこれからの番組を、その場で示す (設計書 4.3)。
export function RulePreview({ input, enabled, onPrograms }: Props) {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [preview, setPreview] = useState<Preview | null>(null);
  const [warnings, setWarnings] = useState<readonly Warning[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [selected, setSelected] = useState<string | undefined>(undefined);
  // 渡されるたびに作り直される関数で取り直さないよう、ref で持つ。
  const onProgramsRef = useRef(onPrograms);
  onProgramsRef.current = onPrograms;
  // 入力の中身が変わったときと、番組表が変わったときだけ取り直す。
  const key = JSON.stringify(input);
  // 番組表が変わったら、一致する番組を取り直す (#867)。
  const epgChanges = useDataChanges(previewChannels);

  useEffect(() => {
    if (!enabled) {
      // 条件を消したら、読み込み中とエラーの表示も消す。読み込み中の応答は、cleanup で捨てている。
      setPreview(null);
      setWarnings([]);
      setLoading(false);
      setError(false);
      onProgramsRef.current?.([]);

      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      setLoading(true);
      setError(false);
      // key から戻した入力で取る。effect は、入力そのものではなく key と番組表の変化を見る。
      const variables = { input: JSON.parse(key) as Props['input'] };
      fetchQuery<RulePreviewQuery>(environment, query, variables, { fetchPolicy: 'network-only' })
        .toPromise()
        .then(
          (data) => {
            if (!cancelled && data !== undefined) {
              setPreview(data.previewRule);
              setWarnings(data.ruleWarnings);
              onProgramsRef.current?.(
                // 同じ番組名の回を見分けられるよう、放送の日時も添える。選択肢の文字は印を描けないので、
                // 囲み文字を外し、全角の英数字は半角にする。
                data.previewRule.edges.map(({ node }) => ({
                  id: node.id,
                  name: `${programTime.format(Date.parse(node.startAt))} ${toHalfWidth(splitProgramMarks(node.name).name)}`,
                })),
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
  }, [key, enabled, environment, epgChanges]);

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
        {enabled && warnings.length > 0 && (
          <Alert color="amber" title={t('rules.preview.slowTitle')}>
            <Stack gap={4}>
              {warnings.map((warning) => (
                <Text key={`${warning.exclude}:${warning.index}`} size="sm">
                  {t(
                    warning.exclude
                      ? 'rules.preview.slowExcludeKeyword'
                      : 'rules.preview.slowKeyword',
                    {
                      text: keywordText(input, warning),
                    },
                  )}
                </Text>
              ))}
              <Text size="sm">{t('rules.preview.slowHint')}</Text>
            </Stack>
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

// keywordText は、警告が指すキーワードの文字列を、入力から引く。
function keywordText(input: Props['input'], warning: Warning): string {
  const keywords = warning.exclude ? input.conditions.excludeKeywords : input.conditions.keywords;

  return keywords?.[warning.index]?.text ?? '';
}
