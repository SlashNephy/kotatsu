import { Alert, Button, Group, Modal, Select, Stack, Text, TextInput } from '@mantine/core';
import { useTimeZoneLabel } from '../futon/timeZone';
import { Suspense, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { ErrorBoundary } from '../futon';
import type { TimeReservationDialogCreateMutation } from './__generated__/TimeReservationDialogCreateMutation.graphql';
import type { TimeReservationDialogQuery } from './__generated__/TimeReservationDialogQuery.graphql';
import type { TimeReservationDialogUpdateMutation } from './__generated__/TimeReservationDialogUpdateMutation.graphql';
import {
  fromInputValue,
  maxNameLength,
  toInputValue,
  validateRange,
  type TimeRangeError,
} from './timeReservation';
import { renderServiceOption, serviceGroups } from '../guide/serviceOption';

// EditableTimeReservation は、時刻と名前を変える時刻予約。サービスは変えられない。
export type EditableTimeReservation = {
  id: string;
  startAt: string;
  endAt: string;
  name: string;
};

type Props = {
  // reservation は、変える時刻予約。undefined なら新しく作る。
  reservation?: EditableTimeReservation;
  onClose: () => void;
  onSaved: () => void;
};

// TimeReservationDialog は、時刻予約を作り、時刻と名前を変えるダイアログ (設計書 4.1)。
export function TimeReservationDialog({ reservation, onClose, onSaved }: Props) {
  const { t } = useTranslation();

  return (
    <Modal
      opened
      onClose={onClose}
      title={t(
        reservation === undefined ? 'reservations.time.createTitle' : 'reservations.time.editTitle',
      )}
    >
      <ErrorBoundary
        fallback={() => <Alert color="amber">{t('reservations.time.loadError')}</Alert>}
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Form reservation={reservation} onClose={onClose} onSaved={onSaved} />
        </Suspense>
      </ErrorBoundary>
    </Modal>
  );
}

const rangeErrorKeys = {
  missing: 'reservations.time.error.missing',
  order: 'reservations.time.error.order',
  tooLong: 'reservations.time.error.tooLong',
} as const satisfies Record<TimeRangeError, string>;

function Form({ reservation, onClose, onSaved }: Props) {
  const { t } = useTranslation();
  // zoneLabel は、時刻を入れるタイムゾーンの呼び方。
  const zoneLabel = useTimeZoneLabel();
  const [create, creating] = useMutation<TimeReservationDialogCreateMutation>(graphql`
    mutation TimeReservationDialogCreateMutation($input: TimeReservationInput!) {
      createTimeReservation(input: $input) {
        id
      }
    }
  `);
  const [update, updating] = useMutation<TimeReservationDialogUpdateMutation>(graphql`
    mutation TimeReservationDialogUpdateMutation($id: ID!, $input: ReservationUpdateInput!) {
      updateReservation(id: $id, input: $input) {
        id
        startAt
        endAt
        name
      }
    }
  `);
  // 新しく作るときは、次の正時から 1 時間を初めの値にする。
  const [initialStart] = useState(() => Math.ceil(Date.now() / 3_600_000) * 3_600_000);
  const [service, setService] = useState<string | null>(null);
  const [start, setStart] = useState(
    reservation !== undefined
      ? toInputValue(Date.parse(reservation.startAt))
      : toInputValue(initialStart),
  );
  const [end, setEnd] = useState(
    reservation !== undefined
      ? toInputValue(Date.parse(reservation.endAt))
      : toInputValue(initialStart + 3_600_000),
  );
  const [name, setName] = useState(reservation?.name ?? '');
  const [error, setError] = useState(false);
  const startMs = fromInputValue(start);
  const endMs = fromInputValue(end);
  const rangeError = validateRange(startMs, endMs);
  const ready =
    rangeError === null &&
    (reservation !== undefined || service !== null) &&
    !creating &&
    !updating;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!ready || startMs === null || endMs === null) {
      return;
    }
    setError(false);
    const times = {
      startAt: new Date(startMs).toISOString(),
      endAt: new Date(endMs).toISOString(),
      name: name.trim(),
    };
    // Relay は、data とともに返ったエラーを、onError ではなく onCompleted に渡す。
    const onCompleted = (_response: unknown, errors: readonly unknown[] | null | undefined) =>
      errors != null && errors.length > 0 ? setError(true) : onSaved();
    if (reservation === undefined) {
      if (service === null) {
        return;
      }
      create({
        variables: { input: { service, ...times } },
        onCompleted,
        onError: () => setError(true),
      });
    } else {
      update({
        variables: { id: reservation.id, input: times },
        onCompleted,
        onError: () => setError(true),
      });
    }
  };

  return (
    <form onSubmit={submit}>
      <Stack>
        {reservation === undefined && (
          // 編集ではサービスを変えられないので、サービスの一覧は、作るときだけ読む。
          <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
            <ServiceSelect value={service} onChange={setService} />
          </Suspense>
        )}
        <TextInput
          type="datetime-local"
          label={t('reservations.time.startAt')}
          value={start}
          onChange={(e) => setStart(e.currentTarget.value)}
          required
        />
        <TextInput
          type="datetime-local"
          label={t('reservations.time.endAt')}
          description={t('reservations.time.endAtDescription', { zone: zoneLabel })}
          value={end}
          onChange={(e) => setEnd(e.currentTarget.value)}
          error={
            rangeError !== null && rangeError !== 'missing'
              ? t(rangeErrorKeys[rangeError])
              : undefined
          }
          required
        />
        <TextInput
          label={t('reservations.time.name')}
          value={name}
          onChange={(e) => setName(e.currentTarget.value)}
          maxLength={maxNameLength}
        />
        {error && (
          <Alert color="red" role="alert">
            {t('reservations.time.saveError')}
          </Alert>
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t('reservations.time.cancel')}
          </Button>
          <Button type="submit" loading={creating || updating} disabled={!ready}>
            {t(reservation === undefined ? 'reservations.time.create' : 'reservations.time.save')}
          </Button>
        </Group>
      </Stack>
    </form>
  );
}

// ServiceSelect は、時刻予約を作るサービスを選ぶ欄。
function ServiceSelect({
  value,
  onChange,
}: {
  value: string | null;
  onChange: (value: string | null) => void;
}) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<TimeReservationDialogQuery>(
    graphql`
      query TimeReservationDialogQuery {
        services {
          id
          name
          logoUrl
          serviceId
          channelType {
            displayName
          }
        }
      }
    `,
    {},
  );

  return (
    <Select
      label={t('reservations.time.service')}
      data={serviceGroups(
        data.services,
        // 同じ名前のサービスが、別のチャンネル種別や、同じ放送局の別のサービスにもあるので、種別とサービス ID を添える。
        (s) => `${s.name}（${s.channelType.displayName}・${s.serviceId}）`,
      )}
      renderOption={renderServiceOption(data.services)}
      value={value}
      onChange={onChange}
      searchable
      required
    />
  );
}
