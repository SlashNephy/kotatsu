import { ActionIcon, Tooltip } from '@mantine/core';
import { Pin, PinOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import type { PinButtonMutation } from './__generated__/PinButtonMutation.graphql';

type Props = {
  serviceId: string;
  serviceName: string;
  pinned: boolean;
  size?: 'xs' | 'sm' | 'md';
};

// PinButton は、サービスをピン留めするか外すボタン (#485)。ピン留めはユーザーごとに覚え、端末をまたいで同期する。
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
  const label = t(pinned ? 'pin.unpin' : 'pin.pin', { name: serviceName });

  return (
    <Tooltip label={label}>
      <ActionIcon
        variant={pinned ? 'light' : 'subtle'}
        color={pinned ? undefined : 'gray'}
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
        {pinned ? <PinOff size={14} aria-hidden /> : <Pin size={14} aria-hidden />}
      </ActionIcon>
    </Tooltip>
  );
}
