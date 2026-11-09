import { ActionIcon, Tooltip } from '@mantine/core';
import { Star } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { toHalfWidth } from '../futon';

import type { PinButtonMutation } from './__generated__/PinButtonMutation.graphql';

const starColor = 'var(--mantine-color-amber-6)';

type Props = {
  serviceId: string;
  serviceName: string;
  pinned: boolean;
  size?: 'xs' | 'sm' | 'md';
};

// PinButton は、サービスをお気に入りに追加するか外す、星のボタン (#485)。画面では「お気に入り」と呼び、コードと API では pinned と呼ぶ。
// お気に入りはユーザーごとに覚え、端末をまたいで同期する。
// 押した後は、ピン留めしたサービスの一覧 (pinnedServices) を、返ってきたものに置き換える。
export function PinButton({ serviceId, serviceName, pinned, size = 'sm' }: Props) {
  const { t } = useTranslation();
  const [commit, inFlight] = useMutation<PinButtonMutation>(graphql`
    mutation PinButtonMutation($id: ID!, $pinned: Boolean!) {
      setServicePinned(id: $id, pinned: $pinned) {
        id
      }
    }
  `);
  const label = t(pinned ? 'pin.unpin' : 'pin.pin', { name: toHalfWidth(serviceName) });

  return (
    <Tooltip label={label}>
      <ActionIcon
        variant="subtle"
        color={pinned ? 'amber' : 'gray'}
        size={size}
        aria-label={label}
        aria-pressed={pinned}
        loading={inFlight}
        onClick={() =>
          commit({
            variables: { id: serviceId, pinned: !pinned },
            updater: (store) => {
              const services = store.getPluralRootField('setServicePinned');
              if (services !== null) {
                store.getRoot().setLinkedRecords(services, 'pinnedServices');
              }
            },
          })
        }
      >
        {/* ボタンの color に任せると、テーマによって茶色や白に寄るので、星の色は両テーマで同じ黄色にする。 */}
        {pinned ? (
          <Star size={14} color={starColor} fill={starColor} aria-hidden />
        ) : (
          <Star size={14} aria-hidden />
        )}
      </ActionIcon>
    </Tooltip>
  );
}
