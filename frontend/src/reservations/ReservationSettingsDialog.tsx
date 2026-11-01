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
import { graphql, useMutation } from 'react-relay';
import { useLazyLoadQuery } from '../relay/useLazyLoadQuery';

import { Authorship, ErrorBoundary } from '../futon';
import { FileNameTemplateField, InheritedFileNameTemplateField } from './FileNameTemplateField';
import { MarginFields } from './MarginFields';
import { PoolSelect } from './PoolSelect';
import type { ReservationSettingsDialogCreateMutation } from './__generated__/ReservationSettingsDialogCreateMutation.graphql';
import type { ReservationSettingsDialogMutation } from './__generated__/ReservationSettingsDialogMutation.graphql';
import type { ReservationSettingsDialogQuery } from './__generated__/ReservationSettingsDialogQuery.graphql';

// joinDefault は、分かれたファイルをつなぐかどうかを上書きしない選択肢の値。
const joinDefault = 'default';

type Props = {
  // id は、設定を変える予約の ID か、設定してから予約する番組の ID。
  id: string;
  onClose: () => void;
  // onReserved は、番組の ID を渡したときに、予約を作れたら呼ぶ。
  onReserved?: () => void;
};

// ReservationSettingsDialog は、予約ごとの録画の設定 (保存先プール、マージン、ファイル名、優先度、つなぐかどうか、
// イベントリレーへの追従) を変える。ルール予約では、ルールの上書きに、さらにこの予約の上書きを重ねる。
// 番組の ID を渡すと、同じ設定を入れてから、番組予約を作る (#463)。
export function ReservationSettingsDialog({ id, onClose, onReserved }: Props) {
  const { t } = useTranslation();

  return (
    <Modal
      opened
      onClose={onClose}
      title={t(
        onReserved !== undefined
          ? 'reservations.settings.createTitle'
          : 'reservations.settings.title',
      )}
    >
      <ErrorBoundary
        fallback={() => <Alert color="amber">{t('reservations.settings.loadError')}</Alert>}
      >
        <Suspense fallback={<Text c="dimmed">{t('viewer.loading')}</Text>}>
          <Form id={id} onClose={onClose} onReserved={onReserved} />
        </Suspense>
      </ErrorBoundary>
    </Modal>
  );
}

function Form({ id, onClose, onReserved }: Props) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<ReservationSettingsDialogQuery>(
    graphql`
      query ReservationSettingsDialogQuery($id: ID!) {
        systemSettings {
          recordingDefaults {
            poolId
            priority
            joinSegments
            detectCm
          }
        }
        storagePoolNames
        node(id: $id) {
          __typename
          ... on Program {
            id
          }
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
                poolId
                startMarginSeconds
                endMarginSeconds
                fileNameTemplate
                priority
                joinSegments
                detectCm
              }
            }
            settings {
              poolId
              fileNameTemplate
              startMarginSeconds
              endMarginSeconds
              priority
              joinSegments
              detectCm
            }
          }
        }
      }
    `,
    { id },
    { fetchPolicy: 'network-only' },
  );
  const [update, updating] = useMutation<ReservationSettingsDialogMutation>(graphql`
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
          detectCm
        }
        # マージンが変わると、チューナーの割り当てが変わることがあるので、一覧の状態も取り直す。
        assignment {
          state
        }
      }
    }
  `);
  // 番組予約を作るときは、番組に予約を結び付け、番組表の枠と番組の詳細に、予約済みを出す。
  // 番組の詳細が読む項目を、すべて返す。返さないと、ストアにない項目を読んで落ちる。
  const [create, creating] = useMutation<ReservationSettingsDialogCreateMutation>(graphql`
    mutation ReservationSettingsDialogCreateMutation(
      $program: ID!
      $settings: RecordingSettingsOverridesInput!
      $followEventRelay: Boolean!
    ) {
      createProgramReservation(
        input: { program: $program, settings: $settings, followEventRelay: $followEventRelay }
      ) {
        id
        enabled
        kind
        assignment {
          state
          plannedStartAt
          plannedEndAt
        }
        rules {
          id
          name
        }
      }
    }
  `);
  const program = data.node?.__typename === 'Program' ? data.node : null;
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
  const [detectCm, setDetectCm] = useState<boolean | null>(reservation?.settings.detectCm ?? null);
  const [poolId, setPoolId] = useState<string | null>(reservation?.settings.poolId ?? null);
  const [followEventRelay, setFollowEventRelay] = useState(reservation?.followEventRelay ?? true);
  const [error, setError] = useState(false);
  if (reservation === null && program === null) {
    return <Alert color="amber">{t('reservations.settings.loadError')}</Alert>;
  }
  // 作るのは番組予約。
  const kind = reservation?.kind ?? 'PROGRAM';
  const inFlight = updating || creating;

  // 録画の設定に使うのは、一致したルールのうち最も古いもの (先頭) の上書き。
  const rule = kind === 'RULE' ? reservation?.rules[0] : undefined;
  // 空の項目が従う値と、その出どころ。ルールに上書きがあればルール、なければ全体の既定値。
  const defaults = data.systemSettings.recordingDefaults;
  const rulePriority = rule?.settings.priority ?? null;
  const ruleJoin = rule?.settings.joinSegments ?? null;
  const ruleDetect = rule?.settings.detectCm ?? null;
  const rulePool = rule?.settings.poolId ?? null;
  const inherited = (fromRule: boolean, value: string) =>
    fromRule && rule !== undefined
      ? t('reservations.settings.inheritedRule', { name: rule.name, value })
      : t('reservations.settings.inheritedDefault', { value });
  const joinLabel = (join: boolean) => t(join ? 'rules.joinSegmentsOn' : 'rules.joinSegmentsOff');
  const detectLabel = (detect: boolean) => t(detect ? 'rules.detectCmOn' : 'rules.detectCmOff');
  const save = () => {
    setError(false);
    const settings = {
      poolId,
      fileNameTemplate: fileNameTemplate === '' ? null : fileNameTemplate,
      startMarginSeconds: margins.start,
      endMarginSeconds: margins.end,
      priority,
      joinSegments,
      detectCm,
    };
    // Relay は、data とともに返ったエラーを、onError ではなく onCompleted に渡す。
    const failed = (errors: readonly unknown[] | null | undefined) =>
      errors != null && errors.length > 0;
    if (program !== null) {
      create({
        variables: { program: program.id, settings, followEventRelay },
        updater: (store) => {
          const created = store.getRootField('createProgramReservation');
          const record = store.get(program.id);
          if (created !== null && record !== undefined && record !== null) {
            record.setLinkedRecord(created, 'reservation');
          }
        },
        onCompleted: (_response, errors) => {
          if (failed(errors)) {
            setError(true);

            return;
          }
          onClose();
          onReserved?.();
        },
        onError: () => setError(true),
      });

      return;
    }
    update({
      // 上書きは丸ごと置き換わるので、すべての項目を送る。
      variables: {
        id,
        settings,
        // イベントリレーへの追従は、番組予約でだけ変える。ルール予約はルールに従い、時刻予約には番組がない。
        followEventRelay: kind === 'PROGRAM' ? followEventRelay : null,
      },
      onCompleted: (_response, errors) => (failed(errors) ? setError(true) : onClose()),
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
        {kind === 'RULE' && (
          <Text size="sm" c="dimmed">
            {t('reservations.settings.ruleNote')}
          </Text>
        )}
        <PoolSelect
          pools={data.storagePoolNames}
          value={poolId}
          onChange={setPoolId}
          inheritedLabel={
            rulePool !== null && rule !== undefined
              ? t('reservations.settings.poolInheritedRule', { name: rule.name, value: rulePool })
              : t('reservations.settings.poolInheritedDefault', {
                  value: defaults.poolId ?? t('admin.settings.poolAuto'),
                })
          }
          disabled={inFlight}
        />
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
            description={t('fileNameTemplate.reservationDescriptionRule', { name: rule.name })}
            value={fileNameTemplate}
            onChange={setFileNameTemplate}
            fallback={rule.settings.fileNameTemplate}
            disabled={inFlight}
          />
        ) : (
          <InheritedFileNameTemplateField
            label={t('fileNameTemplate.reservationLabel')}
            description={t('fileNameTemplate.ruleDescription')}
            value={fileNameTemplate}
            onChange={setFileNameTemplate}
            disabled={inFlight}
          />
        )}
        <NumberInput
          label={t('rules.priority')}
          description={`${t('reservations.settings.priorityRange')}${inherited(
            rulePriority !== null,
            String(rulePriority ?? defaults.priority),
          )}`}
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
            {
              value: joinDefault,
              label:
                ruleJoin !== null && rule !== undefined
                  ? t('reservations.settings.joinInheritedRule', {
                      name: rule.name,
                      value: joinLabel(ruleJoin),
                    })
                  : t('reservations.settings.joinInheritedDefault', {
                      value: joinLabel(defaults.joinSegments),
                    }),
            },
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
        {/* 全体に従うなら、録画を終えるときの全体の設定 (ルール予約ならルールの設定) で決める (#1209)。 */}
        <Select
          label={t('rules.detectCm')}
          data={[
            {
              value: joinDefault,
              label:
                ruleDetect !== null && rule !== undefined
                  ? t('reservations.settings.joinInheritedRule', {
                      name: rule.name,
                      value: detectLabel(ruleDetect),
                    })
                  : t('reservations.settings.joinInheritedDefault', {
                      value: detectLabel(defaults.detectCm),
                    }),
            },
            { value: 'true', label: t('rules.detectCmOn') },
            { value: 'false', label: t('rules.detectCmOff') },
          ]}
          value={detectCm === null ? joinDefault : String(detectCm)}
          onChange={(value) =>
            setDetectCm(value === null || value === joinDefault ? null : value === 'true')
          }
          allowDeselect={false}
          disabled={inFlight}
        />
        {kind === 'PROGRAM' && (
          <Switch
            label={t('rules.followEventRelay')}
            checked={followEventRelay}
            onChange={(e) => setFollowEventRelay(e.currentTarget.checked)}
            disabled={inFlight}
          />
        )}
        {/* ルール予約は neko が作り、変えるので、ユーザーがいなければ「自動」と出す。 */}
        {reservation !== null && (
          <Authorship
            createdAt={reservation.createdAt}
            createdBy={reservation.createdBy}
            updatedAt={reservation.updatedAt}
            updatedBy={reservation.updatedBy}
            automatic={reservation.kind === 'RULE'}
          />
        )}
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
            {t(program !== null ? 'reservations.settings.reserve' : 'reservations.settings.save')}
          </Button>
        </Group>
      </Stack>
    </form>
  );
}
