import { AspectRatio, Box, Group, Stack, Text, UnstyledButton } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';
import { graphql } from 'react-relay';

import { ProgramTitle, zonedFormat } from '../futon';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';
import type { RuleRecordingListQuery } from './__generated__/RuleRecordingListQuery.graphql';
import { RecordingThumbnail } from './RecordingThumbnail';
import classes from './RuleRecordingList.module.css';

const dateTimeFormat = zonedFormat({
  month: 'numeric',
  day: 'numeric',
  weekday: 'short',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

// listSize は、並べる録画の数。見ている録画を除くので、1 つ多く読む。
const listSize = 20;

type Props = {
  ruleId: string;
  ruleName: string;
  // recordingId は、見ている録画。一覧から除く。
  recordingId: string;
};

// RuleRecordingList は、録画の再生の画面の右の列に置く、同じルールで録ったほかの録画の一覧 (設計書 10.3)。
// YouTube の「次の動画」と同じく、押すとその録画の再生へ移る。
export function RuleRecordingList({ ruleId, ruleName, recordingId }: Props) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<RuleRecordingListQuery>(
    graphql`
      query RuleRecordingListQuery($ruleId: ID!, $first: Int!) {
        recordings(first: $first, filter: { ruleId: $ruleId }) {
          edges {
            node {
              id
              name
              startAt
              thumbnailUrl
            }
          }
        }
      }
    `,
    { ruleId, first: listSize + 1 },
  );
  const recordings = data.recordings.edges
    .map((edge) => edge.node)
    .filter((recording) => recording.id !== recordingId)
    .slice(0, listSize);
  // ほかに録画がなければ、見出しごと出さない。
  if (recordings.length === 0) {
    return null;
  }

  return (
    <Stack gap="xs">
      <Text size="sm" fw={600}>
        {t('player.ruleRecordings', { rule: ruleName })}
      </Text>
      <nav aria-label={t('player.ruleRecordings', { rule: ruleName })}>
        <Stack gap={4}>
          {recordings.map((recording) => (
            <UnstyledButton
              key={recording.id}
              className={classes.row}
              renderRoot={(props) => (
                <Link
                  {...props}
                  to="/recordings/$recordingId/play"
                  params={{ recordingId: recording.id }}
                />
              )}
            >
              <Group gap="xs" wrap="nowrap" align="flex-start">
                <Box w={160} style={{ flexShrink: 0 }}>
                  <AspectRatio ratio={16 / 9} className={classes.thumbnail}>
                    <RecordingThumbnail url={recording.thumbnailUrl} />
                  </AspectRatio>
                </Box>
                <Stack gap={4} style={{ minWidth: 0 }}>
                  <Text size="sm" lineClamp={2}>
                    <ProgramTitle name={recording.name} />
                  </Text>
                  <Text size="xs" c="dimmed">
                    {dateTimeFormat.format(Date.parse(recording.startAt))}
                  </Text>
                </Stack>
              </Group>
            </UnstyledButton>
          ))}
        </Stack>
      </nav>
    </Stack>
  );
}
