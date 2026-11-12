import {
  Alert,
  Badge,
  Button,
  Chip,
  Code,
  Group,
  ScrollArea,
  Stack,
  Switch,
  Text,
  TextInput,
} from '@mantine/core';
import { createFileRoute } from '@tanstack/react-router';
import { memo, startTransition, useDeferredValue, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';

import { AdminPage } from '../../admin/AdminPage';
import { Section } from '../../admin/Section';
import { environmentText, otherRoles } from '../../admin/logs/environment';
import { IssueDialog } from '../../admin/logs/IssueDialog';
import { redact } from '../../admin/logs/redact';
import {
  formatTime,
  fullLogs,
  limitFields,
  logsBudget,
  selectLogs,
  type IssueFields,
  type Level,
  type ReportLine,
  type ReportSource,
} from '../../admin/logs/report';
import { filterLines, mergeLines, pageSize, type ViewLine } from '../../admin/logs/view';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';
import type { logsQuery, logsQuery$data } from './__generated__/logsQuery.graphql';
import classes from './logs.module.css';

export const Route = createFileRoute('/admin/logs')({
  component: LogsPage,
});

// LogsPage は、直近のログを読み、秘密と家の構成を伏せた不具合の報告を作る、管理者向けの画面 (#1521、設計書 10.3、11.3)。
// ログにはユーザー名や番組名が入るので、管理者だけが見られる。サーバーからは GitHub に何も送らない。
function LogsPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.logs.title')}>
      <Logs />
    </AdminPage>
  );
}

const query = graphql`
  query logsQuery {
    version {
      version
    }
    logReport {
      sources {
        kind
        total
        failure
        version
        os
        arch
        runtime
        lines {
          time
          level
          inferred
          text
        }
      }
      redactHosts
      instanceId
      roles
    }
    systemStatus {
      tuners {
        name
        types
      }
    }
  }
`;

type Source = logsQuery$data['logReport']['sources'][number];

// levelOf は、GraphQL の重さを、報告の重さにする。知らない重さ (新しい版のサーバー) は info として扱う。
const levelOf = (level: string): Level => (level === 'ERROR' || level === 'WARN' ? level : 'INFO');

// linesOf は、出どころの行を、画面と報告で使う形にする。
const linesOf = (source: Source): ReportLine[] =>
  source.lines.map((line) => ({ ...line, level: levelOf(line.level) }));

// failureKey は、ログを取れなかった理由の文言のキー。
function failureKey(failure: string) {
  switch (failure) {
    case 'TIMEOUT':
      return 'admin.logs.failures.TIMEOUT' as const;
    case 'UNSUPPORTED':
      return 'admin.logs.failures.UNSUPPORTED' as const;
    case 'HTTP_ERROR':
      return 'admin.logs.failures.HTTP_ERROR' as const;
    default:
      return 'admin.logs.failures.UNREACHABLE' as const;
  }
}

function Logs() {
  const { t } = useTranslation();
  const [fetchKey, setFetchKey] = useState(0);
  const data = useLazyLoadQuery<logsQuery>(query, {}, { fetchKey, fetchPolicy: 'network-only' });
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  const report = data.logReport;

  const [hidden, setHidden] = useState<ReadonlySet<number>>(new Set());
  const [problemsOnly, setProblemsOnly] = useState(false);
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search);
  const [shown, setShown] = useState(pageSize);
  const [issue, setIssue] = useState<{ initial: IssueFields; full: IssueFields } | null>(null);

  const sourceName = (source: Source) =>
    source.kind === 'KOTATSU'
      ? t('admin.logs.sourceKotatsu', { instance: report.instanceId.slice(0, 8) })
      : t('admin.logs.sourceTunerServer');
  const merged = useMemo(
    () => mergeLines(report.sources.map((source) => ({ lines: linesOf(source) }))),
    [report.sources],
  );
  const filtered = useMemo(
    () => filterLines(merged, { hidden, problemsOnly, search: deferredSearch }),
    [merged, hidden, problemsOnly, deferredSearch],
  );
  // 絞り方を変えたら、最初の行数に戻す。
  const changeFilter = (change: () => void) => {
    change();
    setShown(pageSize);
  };
  const others = otherRoles(report.roles);
  const environment = environmentText({
    sources: report.sources,
    roles: report.roles,
    tuners: data.systemStatus.tuners,
    userAgent: navigator.userAgent,
  });

  // openIssue は、伏せた後の欄で、確認の窓を開く。ホスト名は画面ではそのまま出し、報告にだけ伏せる (2 段目)。
  const openIssue = () => {
    const options = { secrets: [], hosts: report.redactHosts };
    const sources: ReportSource[] = report.sources.map((source) => ({
      name:
        source.kind === 'KOTATSU'
          ? `Kotatsu (instance ${report.instanceId}, ${report.roles.join(', ')})`
          : sourceName(source),
      total: source.total,
      failure: source.failure == null ? null : t(failureKey(source.failure)),
      lines: linesOf(source).map((line) => ({ ...line, text: redact(line.text, options) })),
    }));
    const head = limitFields({
      title: '',
      version: redact(data.version.version, options),
      environment: redact(environment, options),
      logs: '',
    });
    setIssue({
      initial: { ...head, logs: selectLogs(sources, logsBudget(head)) },
      full: { ...head, logs: fullLogs(sources) },
    });
  };

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Text size="sm" c="dimmed">
          {t('admin.logs.description')}
        </Text>
        <Group gap="xs">
          <Button size="compact-sm" variant="default" onClick={refresh}>
            {t('admin.logs.refresh')}
          </Button>
          <Button size="compact-sm" onClick={openIssue}>
            {t('admin.logs.reportIssue')}
          </Button>
        </Group>
      </Group>

      {others.length > 0 && (
        <Alert color="blue" title={t('admin.logs.splitTitle', { instance: report.instanceId })}>
          <Stack gap={4}>
            <Text size="sm">
              {t('admin.logs.splitBody', {
                roles: report.roles.join(', '),
                others: others.join(', '),
              })}
            </Text>
            <Code block>
              {others
                .map((role) => `kubectl logs -l app.kubernetes.io/component=${role} --tail=1000`)
                .join('\n')}
            </Code>
          </Stack>
        </Alert>
      )}

      <Section title={t('admin.logs.environment')}>
        <Code block>{environment}</Code>
      </Section>

      <Section title={t('admin.logs.lines')}>
        <Stack gap="xs">
          <Group gap="xs">
            {report.sources.map((source, index) => (
              <Chip
                key={source.kind}
                size="xs"
                checked={!hidden.has(index)}
                onChange={(checked) =>
                  changeFilter(() => {
                    const next = new Set(hidden);
                    if (checked) {
                      next.delete(index);
                    } else {
                      next.add(index);
                    }
                    setHidden(next);
                  })
                }
              >
                {t('admin.logs.sourceCount', {
                  name: sourceName(source),
                  count: source.lines.length,
                  total: source.total,
                })}
              </Chip>
            ))}
            <Switch
              size="sm"
              label={t('admin.logs.problemsOnly')}
              checked={problemsOnly}
              onChange={(event) => {
                const checked = event.currentTarget.checked;
                changeFilter(() => setProblemsOnly(checked));
              }}
            />
          </Group>
          <TextInput
            size="xs"
            placeholder={t('admin.logs.search')}
            aria-label={t('admin.logs.search')}
            value={search}
            onChange={(event) => {
              const value = event.currentTarget.value;
              changeFilter(() => setSearch(value));
            }}
          />
          {report.sources
            .filter((source) => source.failure != null)
            .map((source) => (
              <Alert key={source.kind} color="amber" p="xs">
                <Text size="sm">
                  {t('admin.logs.failed', {
                    name: sourceName(source),
                    reason: t(failureKey(source.failure ?? '')),
                  })}
                </Text>
              </Alert>
            ))}
          {filtered.length === 0 ? (
            <Text size="sm" c="dimmed">
              {t('admin.logs.empty')}
            </Text>
          ) : (
            <ScrollArea.Autosize mah="70vh" type="auto">
              <Stack gap={0} data-testid="log-lines">
                {filtered.slice(0, shown).map((line, index) => (
                  <LogRow
                    key={`${line.source}-${line.time}-${index}`}
                    line={line}
                    source={
                      report.sources.length > 1
                        ? report.sources[line.source]?.kind === 'KOTATSU'
                          ? 'Kotatsu'
                          : t('admin.logs.tunerShort')
                        : null
                    }
                  />
                ))}
              </Stack>
            </ScrollArea.Autosize>
          )}
          <Group justify="space-between">
            <Text size="xs" c="dimmed">
              {t('admin.logs.showing', {
                shown: Math.min(shown, filtered.length),
                count: filtered.length,
              })}
            </Text>
            {shown < filtered.length && (
              <Button
                size="compact-xs"
                variant="default"
                onClick={() => setShown(shown + pageSize)}
              >
                {t('admin.logs.more')}
              </Button>
            )}
          </Group>
        </Stack>
      </Section>

      {issue != null && (
        <IssueDialog initial={issue.initial} full={issue.full} onClose={() => setIssue(null)} />
      )}
    </Stack>
  );
}

// LogRow は、ログの 1 行。「もっと見る」で行を足したときに、描いてある行を描き直さないよう、memo にする。
const LogRow = memo(function LogRow({ line, source }: { line: ViewLine; source: string | null }) {
  const { t } = useTranslation();

  return (
    <div className={classes.row}>
      <span className={classes.time}>{formatTime(line.time)}</span>
      {source != null && (
        <Badge size="xs" variant="outline" color="gray" className={classes.badge}>
          {source}
        </Badge>
      )}
      {line.level !== 'INFO' && (
        <Badge
          size="xs"
          variant={line.inferred ? 'outline' : 'filled'}
          color={line.level === 'ERROR' ? 'red' : 'amber'}
          title={line.inferred ? t('admin.logs.inferred') : undefined}
          className={classes.badge}
        >
          {line.level === 'ERROR' ? t('admin.logs.levelError') : t('admin.logs.levelWarn')}
        </Badge>
      )}
      <span className={classes.text}>{line.text}</span>
    </div>
  );
});
