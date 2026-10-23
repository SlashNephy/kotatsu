import { ActionIcon, Tooltip } from '@mantine/core';
import { Bell, BellRing } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import type { WatchReserveButtonMutation } from './__generated__/WatchReserveButtonMutation.graphql';

type Props = {
  programId: string;
  reserved: boolean;
  // size は、横に並べるボタンと高さをそろえる大きさ。
  size?: 'md' | 'lg';
};

// WatchReserveButton は、番組の視聴予約を付け外しするベルのボタン (設計書 4.8)。
// 録画はせず、放送の少し前に、押したユーザーだけに通知する。閲覧だけのユーザーも使える。
export function WatchReserveButton({ programId, reserved, size = 'lg' }: Props) {
  const { t } = useTranslation();
  const [error, setError] = useState(false);
  const [commit, inFlight] = useMutation<WatchReserveButtonMutation>(graphql`
    mutation WatchReserveButtonMutation($program: ID!, $reserved: Boolean!) {
      # 同じ番組の watchReserved をストアで書き換え、番組表の枠のベルにも反映する。
      setWatchReservation(program: $program, reserved: $reserved) {
        id
        watchReserved
      }
    }
  `);
  const toggle = () => {
    setError(false);
    commit({
      variables: { program: programId, reserved: !reserved },
      onCompleted: (_response, errors) => setError(errors != null && errors.length > 0),
      onError: () => setError(true),
    });
  };
  const label = error ? t('watch.error') : reserved ? t('watch.cancel') : t('watch.reserve');

  return (
    <Tooltip label={label} opened={error ? true : undefined} color={error ? 'red' : undefined}>
      <ActionIcon
        variant={reserved ? 'filled' : 'default'}
        color={reserved ? 'mikan' : undefined}
        size={size === 'lg' ? 36 : 28}
        onClick={toggle}
        loading={inFlight}
        aria-label={reserved ? t('watch.cancel') : t('watch.reserve')}
        aria-pressed={reserved}
      >
        {reserved ? <BellRing size={18} aria-hidden /> : <Bell size={18} aria-hidden />}
      </ActionIcon>
    </Tooltip>
  );
}
