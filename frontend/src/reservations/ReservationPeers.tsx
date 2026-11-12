import { Text, type TextProps } from '@mantine/core';
import { useTranslation } from 'react-i18next';
import { graphql, useFragment } from 'react-relay';

import { splitProgramMarks, toHalfWidth } from '../futon';
import type { ReservationPeers_assignment$key } from './__generated__/ReservationPeers_assignment.graphql';

// ReservationPeers は、競合か一部のみの予約が、チューナーを取り合っている相手の予約を、番組名と局名で並べる (#1502、設計書 4.5)。
// どの予約を外せば録れるかが分かるよう、ただ時間が重なるだけの予約ではなく、neko の割り当てで取り合っている予約だけを出す。
// 相手のない予約では、何も描かない。
export function ReservationPeers({
  assignment,
  ...props
}: { assignment: ReservationPeers_assignment$key } & TextProps) {
  const { t } = useTranslation();
  const { peers } = useFragment(
    graphql`
      fragment ReservationPeers_assignment on ReservationAssignment {
        peers {
          id
          name
          service {
            name
          }
        }
      }
    `,
    assignment,
  );
  if (peers.length === 0) {
    return null;
  }
  const names = peers
    .map((peer) =>
      t('reservations.peer', {
        // 一覧の番組名と同じく半角にそろえ、並べて読みやすいよう囲み文字は外す。
        name:
          peer.name === ''
            ? t('reservations.untitled')
            : toHalfWidth(splitProgramMarks(peer.name).name),
        service: toHalfWidth(peer.service.name),
      }),
    )
    .join(t('reservations.peerSeparator'));

  return (
    <Text size="xs" c="amber.8" {...props}>
      {t('reservations.peers', { peers: names })}
    </Text>
  );
}
