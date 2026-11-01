import { ActionIcon, Tooltip } from '@mantine/core';
import { X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { RecordingCard, type RecordingCardData } from '../recordings/RecordingCard';
import { splitProgramMarks, toHalfWidth } from '../futon';
import classes from './ContinueWatchingCard.module.css';

import type { ContinueWatchingCardMutation } from './__generated__/ContinueWatchingCardMutation.graphql';

type Props = {
  recording: RecordingCardData;
  // first は、ホームが読んだ続きから見るの数。外した録画を、同じ引数の一覧から除く。
  first: number;
};

// ContinueWatchingCard は、続きから見るの録画のカードに、一覧から外すボタンを重ねる (#1074)。
// 外すと、ユーザーの再生位置を消すので、次は始まりから再生する。
// カードはリンクなので、ボタンはカードの中ではなく上に重ねる (PinnableCard と同じ)。
export function ContinueWatchingCard({ recording, first }: Props) {
  const { t } = useTranslation();
  const [commit, inFlight] = useMutation<ContinueWatchingCardMutation>(graphql`
    mutation ContinueWatchingCardMutation($id: ID!) {
      clearWatchPosition(id: $id) {
        id
        watchPositionMs
        watchFrame {
          url
        }
      }
    }
  `);
  const label = t('home.removeContinueWatching', {
    name: toHalfWidth(splitProgramMarks(recording.name).name),
  });

  return (
    <div className={classes.root}>
      <RecordingCard recording={recording} />
      <div className={classes.remove}>
        <Tooltip label={label}>
          <ActionIcon
            variant="filled"
            color="dark"
            size="sm"
            radius="xl"
            aria-label={label}
            loading={inFlight}
            onClick={() =>
              commit({
                variables: { id: recording.id },
                updater: (store) => {
                  const root = store.getRoot();
                  const records = root.getLinkedRecords('continueWatching', { first });
                  if (records != null) {
                    root.setLinkedRecords(
                      records.filter((record) => record?.getDataID() !== recording.id),
                      'continueWatching',
                      { first },
                    );
                  }
                },
              })
            }
          >
            <X size={14} aria-hidden />
          </ActionIcon>
        </Tooltip>
      </div>
    </div>
  );
}
