import {
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Code,
  Group,
  ScrollArea,
  Stack,
  Table,
  Tabs,
  Text,
} from '@mantine/core';
import { ListChecks } from 'lucide-react';
import { createFileRoute, Link } from '@tanstack/react-router';
import { Fragment, startTransition, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useRelayEnvironment } from 'react-relay';

import { AdminPage } from '../../admin/AdminPage';
import { useAction } from '../../admin/useAction';
import { EmptyState, zonedFormat } from '../../futon';
import { jobKindLabel, jobStates, type JobStateName } from '../../admin/jobs';
import { useLazyLoadQuery } from '../../relay/useLazyLoadQuery';
import { settingsErrorMessage } from '../../settings/errors';
import { mutate } from '../../settings/mutate';
import type { jobsCancelMutation } from './__generated__/jobsCancelMutation.graphql';
import type { jobsQuery, JobState } from './__generated__/jobsQuery.graphql';
import type { jobsRetryMutation } from './__generated__/jobsRetryMutation.graphql';

export const Route = createFileRoute('/admin/jobs')({
  component: JobsPage,
});

// refreshInterval は、一覧と数を読み直す間隔 (ミリ秒)。River UI と同じく、動いている様子を追えるようにする。
const refreshInterval = 5_000;

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hourCycle: 'h23',
});

// JobsPage は、nekonote の River のジョブを、状態ごとに確かめる管理者向けの画面 (#1279、設計書 6.1)。
// River UI の構成を模し、状態のタブと数、ジョブの一覧、ジョブを開いたときのエラーと操作を並べる。
function JobsPage() {
  const { t } = useTranslation();

  return (
    <AdminPage title={t('admin.jobs.title')}>
      <Jobs />
    </AdminPage>
  );
}

const query = graphql`
  query jobsQuery($state: JobState!) {
    jobCounts {
      state
      count
    }
    jobs(state: $state, first: 100) {
      nodes {
        id
        kind
        state
        queue
        attempt
        maxAttempts
        createdAt
        scheduledAt
        attemptedAt
        finalizedAt
        retryable
        cancellable
        errors {
          at
          attempt
          message
        }
        recording {
          id
          name
        }
      }
    }
  }
`;

function Jobs() {
  const { t } = useTranslation();
  const environment = useRelayEnvironment();
  const [state, setState] = useState<JobStateName>('RUNNING');
  const [fetchKey, setFetchKey] = useState(0);
  const [opened, setOpened] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { run, busy } = useAction();
  const data = useLazyLoadQuery<jobsQuery>(
    query,
    { state: state as JobState },
    { fetchKey, fetchPolicy: 'store-and-network' },
  );
  const refresh = () => startTransition(() => setFetchKey((key) => key + 1));
  useEffect(() => {
    const timer = window.setInterval(refresh, refreshInterval);

    return () => window.clearInterval(timer);
  }, []);
  const counts = new Map(data.jobCounts.map((count) => [count.state, count.count]));
  const fail = (caught: unknown) => setError(settingsErrorMessage(t, caught));
  const retry = (id: string) =>
    run(async () => {
      setError(null);
      await mutate<jobsRetryMutation>(
        environment,
        graphql`
          mutation jobsRetryMutation($id: ID!) {
            retryJob(id: $id) {
              id
              state
            }
          }
        `,
        { id },
      );
      refresh();
    }, fail);
  const cancel = (id: string) =>
    run(async () => {
      setError(null);
      await mutate<jobsCancelMutation>(
        environment,
        graphql`
          mutation jobsCancelMutation($id: ID!) {
            cancelJob(id: $id) {
              id
              state
            }
          }
        `,
        { id },
      );
      refresh();
    }, fail);

  return (
    <Stack gap="md">
      <Text size="sm" c="dimmed">
        {t('admin.jobs.description')}
      </Text>
      {error !== null && (
        <Alert color="red" role="alert" withCloseButton onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      <Tabs
        value={state}
        onChange={(value) => {
          if (value !== null) {
            setOpened(null);
            startTransition(() => setState(value as JobStateName));
          }
        }}
      >
        <ScrollArea type="auto" offsetScrollbars>
          <Tabs.List style={{ flexWrap: 'nowrap' }}>
            {jobStates.map((name) => (
              <Tabs.Tab
                key={name}
                value={name}
                rightSection={
                  <Badge size="sm" variant="light" color="gray">
                    {counts.get(name) ?? 0}
                  </Badge>
                }
              >
                {t(`admin.jobs.states.${name}`)}
              </Tabs.Tab>
            ))}
          </Tabs.List>
        </ScrollArea>
      </Tabs>
      {data.jobs.nodes.length === 0 ? (
        <EmptyState icon={ListChecks} title={t('admin.jobs.empty')} />
      ) : (
        <Card withBorder p={0}>
          <ScrollArea type="auto">
            <Table highlightOnHover verticalSpacing="xs" miw={720}>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>{t('admin.jobs.id')}</Table.Th>
                  <Table.Th>{t('admin.jobs.kind')}</Table.Th>
                  <Table.Th>{t('admin.jobs.target')}</Table.Th>
                  <Table.Th>{t('admin.jobs.attempts')}</Table.Th>
                  <Table.Th>{t('admin.jobs.time')}</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {data.jobs.nodes.map((job) => {
                  const last = job.errors.at(-1);
                  const recording = job.recording;
                  const time = job.finalizedAt ?? job.attemptedAt ?? job.scheduledAt;

                  return (
                    <Fragment key={job.id}>
                      <Table.Tr
                        style={{ cursor: 'pointer' }}
                        onClick={() => setOpened(opened === job.id ? null : job.id)}
                        aria-expanded={opened === job.id}
                      >
                        <Table.Td>
                          <Code>{job.id}</Code>
                        </Table.Td>
                        <Table.Td>
                          <Stack gap={0}>
                            <Text size="sm">{jobKindLabel(t, job.kind)}</Text>
                            <Text size="xs" c="dimmed">
                              {job.kind} ・ {job.queue}
                            </Text>
                          </Stack>
                        </Table.Td>
                        <Table.Td>
                          {recording != null ? (
                            <Anchor
                              size="sm"
                              onClick={(event) => event.stopPropagation()}
                              renderRoot={(props) => (
                                <Link
                                  {...props}
                                  to="/recordings/$recordingId"
                                  params={{ recordingId: recording.id }}
                                />
                              )}
                            >
                              {recording.name}
                            </Anchor>
                          ) : (
                            <Text size="sm" c="dimmed">
                              -
                            </Text>
                          )}
                        </Table.Td>
                        <Table.Td>
                          <Text size="sm">
                            {job.attempt} / {job.maxAttempts}
                          </Text>
                          {last !== undefined && (
                            <Text size="xs" c="red" lineClamp={1} maw={260}>
                              {last.message}
                            </Text>
                          )}
                        </Table.Td>
                        <Table.Td>
                          <Text size="sm">{dateTimeFormat.format(Date.parse(time))}</Text>
                        </Table.Td>
                      </Table.Tr>
                      {opened === job.id && (
                        <Table.Tr>
                          <Table.Td colSpan={5}>
                            <Stack gap="xs">
                              <Text size="xs" c="dimmed">
                                {t('admin.jobs.times', {
                                  created: dateTimeFormat.format(Date.parse(job.createdAt)),
                                  scheduled: dateTimeFormat.format(Date.parse(job.scheduledAt)),
                                })}
                              </Text>
                              {job.errors.length === 0 ? (
                                <Text size="sm" c="dimmed">
                                  {t('admin.jobs.noErrors')}
                                </Text>
                              ) : (
                                job.errors.map((e) => (
                                  <Stack key={`${e.attempt}-${e.at}`} gap={2}>
                                    <Text size="xs" c="dimmed">
                                      {t('admin.jobs.errorAt', {
                                        attempt: e.attempt,
                                        at: dateTimeFormat.format(Date.parse(e.at)),
                                      })}
                                    </Text>
                                    <Code block style={{ whiteSpace: 'pre-wrap' }}>
                                      {e.message}
                                    </Code>
                                  </Stack>
                                ))
                              )}
                              <Group gap="xs">
                                <Button
                                  size="compact-sm"
                                  variant="default"
                                  disabled={!job.retryable || busy}
                                  onClick={() => retry(job.id)}
                                >
                                  {t('admin.jobs.retry')}
                                </Button>
                                <Button
                                  size="compact-sm"
                                  variant="default"
                                  color="red"
                                  disabled={!job.cancellable || busy}
                                  onClick={() => cancel(job.id)}
                                >
                                  {t('admin.jobs.cancel')}
                                </Button>
                                {!job.cancellable && job.finalizedAt == null && (
                                  <Text size="xs" c="dimmed">
                                    {t('admin.jobs.uncancellable')}
                                  </Text>
                                )}
                              </Group>
                            </Stack>
                          </Table.Td>
                        </Table.Tr>
                      )}
                    </Fragment>
                  );
                })}
              </Table.Tbody>
            </Table>
          </ScrollArea>
        </Card>
      )}
    </Stack>
  );
}
