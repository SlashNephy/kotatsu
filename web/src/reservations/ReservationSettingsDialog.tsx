import {
  Alert,
  Button,
  Group,
  Modal,
  NumberInput,
  Select,
  Stack,
  Switch,
  Text,
} from '@mantine/core';
import { Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery, useMutation } from 'react-relay';

import { Authorship, ErrorBoundary } from '../futon';
import { FileNameTemplateField, InheritedFileNameTemplateField } from './FileNameTemplateField';
import { MarginFields } from './MarginFields';
import type { ReservationSettingsDialogMutation } from './__generated__/ReservationSettingsDialogMutation.graphql';
import type { ReservationSettingsDialogQuery } from './__generated__/ReservationSettingsDialogQuery.graphql';

// joinDefault は、分かれたファイルをつなぐかどうかを上書きしない選択肢の値。
const joinDefault = 'default';

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
            followEventRelay
            createdAt
            createdBy {
              displayName
            }
            updatedAt
            updatedBy {
              displayName
            }
            rules {
              name
              settings {
                startMarginSeconds
                endMarginSeconds
                fileNameTemplate
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
      $followEventRelay: Boolean
    ) {
      updateReservation(
        id: $id
        input: { settings: $settings, followEventRelay: $followEventRelay }
      ) {
        id
        followEventRelay
        settings {
          poolId
          fileNameTemplate
          startMarginSeconds
          endMarginSeconds
          priority
          joinSegments
        }
        # マージンが変わると、チューナーの割り当てが変わることがあるので、一覧の状態も取り直す。
        assignment {
          state
        }
      }
    }
  `);
  const reservation = data.node?.__typename === 'Reservation' ? data.node : null;
  const [margins, setMargins] = useState({
    start: reservation?.settings.startMarginSeconds ?? null,
    end: reservation?.settings.endMarginSeconds ?? null,
  });
  const [fileNameTemplate, setFileNameTemplate] = useState(
    reservation?.settings.fileNameTemplate ?? '',
  );
  const [priority, setPriority] = useState<number | null>(reservation?.settings.priority ?? null);
  const [joinSegments, setJoinSegments] = useState<boolean | null>(
    reservation?.settings.joinSegments ?? null,
  );
  const [followEventRelay, setFollowEventRelay] = useState(reservation?.followEventRelay ?? true);
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
      // 上書きは丸ごと置き換わるので、ここで変えない項目 (保存先プール) もそのまま送る。
      variables: {
        id: reservationId,
        settings: {
          poolId: s.poolId ?? null,
          fileNameTemplate: fileNameTemplate === '' ? null : fileNameTemplate,
          startMarginSeconds: margins.start,
          endMarginSeconds: margins.end,
          priority,
          joinSegments,
        },
        // イベントリレーへの追従は、番組予約でだけ変える。ルール予約はルールに従い、時刻予約には番組がない。
        followEventRelay: reservation.kind === 'PROGRAM' ? followEventRelay : null,
      },
      onCompleted: (_response, errors) =>
        errors != null && errors.length > 0 ? setError(true) : onClose(),
      onError: () => setError(true),
    });
  };

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        save();
      }}
    >
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
        {rule?.settings.fileNameTemplate != null ? (
          // ルールのテンプレートがあれば、空のときはそれに従う。
          <FileNameTemplateField
            label={t('fileNameTemplate.reservationLabel')}
            description={t('fileNameTemplate.reservationDescription')}
            value={fileNameTemplate}
            onChange={setFileNameTemplate}
            fallback={rule.settings.fileNameTemplate}
            disabled={inFlight}
          />
        ) : (
          <InheritedFileNameTemplateField
            label={t('fileNameTemplate.reservationLabel')}
            description={t('fileNameTemplate.reservationDescription')}
            value={fileNameTemplate}
            onChange={setFileNameTemplate}
            disabled={inFlight}
          />
        )}
        <NumberInput
          label={t('rules.priority')}
          description={t('reservations.settings.priorityDescription')}
          value={priority ?? ''}
          onChange={(value) => setPriority(typeof value === 'number' ? value : null)}
          min={-10}
          max={10}
          allowDecimal={false}
          disabled={inFlight}
        />
        <Select
          label={t('rules.joinSegments')}
          data={[
            { value: joinDefault, label: t('reservations.settings.joinSegmentsDefault') },
            { value: 'true', label: t('rules.joinSegmentsOn') },
            { value: 'false', label: t('rules.joinSegmentsOff') },
          ]}
          value={joinSegments === null ? joinDefault : String(joinSegments)}
          onChange={(value) =>
            setJoinSegments(value === null || value === joinDefault ? null : value === 'true')
          }
          allowDeselect={false}
          disabled={inFlight}
        />
        {reservation.kind === 'PROGRAM' && (
          <Switch
            label={t('rules.followEventRelay')}
            checked={followEventRelay}
            onChange={(e) => setFollowEventRelay(e.currentTarget.checked)}
            disabled={inFlight}
          />
        )}
        {/* ルール予約は neko が作り、変えるので、ユーザーがいなければ「自動」と出す。 */}
        <Authorship
          createdAt={reservation.createdAt}
          createdBy={reservation.createdBy}
          updatedAt={reservation.updatedAt}
          updatedBy={reservation.updatedBy}
          automatic={reservation.kind === 'RULE'}
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
          <Button type="submit" loading={inFlight}>
            {t('reservations.settings.save')}
          </Button>
        </Group>
      </Stack>
    </form>
  );
}
