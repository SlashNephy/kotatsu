import { Alert, Button, Group, Modal, Stack, Text } from '@mantine/core';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { ErrorBoundary } from '../futon';
import { MarginFields } from './MarginFields';
import type { ReservationSettingsDialogMutation } from './__generated__/ReservationSettingsDialogMutation.graphql';
import type { ReservationSettingsDialogQuery } from './__generated__/ReservationSettingsDialogQuery.graphql';

type Props = {
  reservationId: string;
  onClose: () => void;
};

// ReservationSettingsDialog は、予約ごとの録画の設定 (前後のマージン) を変える。
// ルール予約では、ルールの上書きに、さらにこの予約の上書きを重ねる。
export function ReservationSettingsDialog({ reservationId, onClose }: Props) {
  const { t } = useTranslation();

  return (
    <Modal opened onClose={onClose} title={t('reservations.settings.title')}>
      <ErrorBoundary
        fallback={() => <Alert color="amber">{t('reservations.settings.loadError')}</Alert>}
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Form reservationId={reservationId} onClose={onClose} />
        </Suspense>
      </ErrorBoundary>
    </Modal>
  );
}

function Form({ reservationId, onClose }: Props) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<ReservationSettingsDialogQuery>(
    graphql`
      query ReservationSettingsDialogQuery($id: ID!) {
        node(id: $id) {
          __typename
          ... on Reservation {
            kind
            rules {
              name
              settings {
                startMarginSeconds
                endMarginSeconds
              }
            }
            settings {
              poolId
              fileNameTemplate
              startMarginSeconds
              endMarginSeconds
              priority
              joinSegments
            }
          }
        }
      }
    `,
    { id: reservationId },
    { fetchPolicy: 'network-only' },
  );
  const [commit, inFlight] = useMutation<ReservationSettingsDialogMutation>(graphql`
    mutation ReservationSettingsDialogMutation(
      $id: ID!
      $settings: RecordingSettingsOverridesInput!
    ) {
      updateReservation(id: $id, input: { settings: $settings }) {
        id
        settings {
          startMarginSeconds
          endMarginSeconds
        }
      }
    }
  `);
  const reservation = data.node?.__typename === 'Reservation' ? data.node : null;
  const [margins, setMargins] = useState({
    start: reservation?.settings.startMarginSeconds ?? null,
    end: reservation?.settings.endMarginSeconds ?? null,
  });
  const [error, setError] = useState(false);
  if (reservation === null) {
    return <Alert color="amber">{t('reservations.settings.loadError')}</Alert>;
  }

  // 録画の設定に使うのは、一致したルールのうち最も古いもの (先頭) の上書き。
  const rule = reservation.kind === 'RULE' ? reservation.rules[0] : undefined;
  const save = () => {
    setError(false);
    const s = reservation.settings;
    commit({
      // 上書きは丸ごと置き換わるので、ここで変えない項目もそのまま送る。
      variables: {
        id: reservationId,
        settings: {
          poolId: s.poolId ?? null,
          fileNameTemplate: s.fileNameTemplate ?? null,
          startMarginSeconds: margins.start,
          endMarginSeconds: margins.end,
          priority: s.priority ?? null,
          joinSegments: s.joinSegments ?? null,
        },
      },
      onCompleted: (_response, errors) =>
        errors != null && errors.length > 0 ? setError(true) : onClose(),
      onError: () => setError(true),
    });
  };

  return (
    <Stack>
      {reservation.kind === 'RULE' && (
        <Text size="sm" c="dimmed">
          {t('reservations.settings.ruleNote')}
        </Text>
      )}
      <MarginFields
        start={margins.start}
        end={margins.end}
        onChange={setMargins}
        disabled={inFlight}
        rule={
          rule === undefined
            ? undefined
            : {
                name: rule.name,
                start: rule.settings.startMarginSeconds ?? null,
                end: rule.settings.endMarginSeconds ?? null,
              }
        }
      />
      {error && (
        <Alert color="red" role="alert">
          {t('reservations.settings.saveError')}
        </Alert>
      )}
      <Group justify="flex-end">
        <Button variant="default" onClick={onClose}>
          {t('reservations.settings.cancel')}
        </Button>
        <Button onClick={save} loading={inFlight}>
          {t('reservations.settings.save')}
        </Button>
      </Group>
    </Stack>
  );
}
