import { Card, Stack, Text } from '@mantine/core';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import { AutoSaveNumberInput } from '../settings/AutoSaveFields';
import { useAutoSave } from '../settings/useAutoSave';
import type { WatchReminderSettingsMutation } from './__generated__/WatchReminderSettingsMutation.graphql';

// maxMinutes は、視聴予約の通知を、放送の何分前に送るかの上限 (設計書 4.8、付録 A)。
const maxMinutes = 60;

// WatchReminderSettings は、視聴予約の通知を、放送の何分前に送るかの設定 (設計書 4.8)。
// 入力欄を離れたときか Enter で保存し、ほかの設定と同じ通知で知らせる (#1740)。
export function WatchReminderSettings({ minutesBefore }: { minutesBefore: number }) {
  const { t } = useTranslation();
  const { save } = useAutoSave();
  const [commit] = useMutation<WatchReminderSettingsMutation>(graphql`
    mutation WatchReminderSettingsMutation($minutesBefore: Int!) {
      updateWatchReminderSettings(minutesBefore: $minutesBefore) {
        minutesBefore
      }
    }
  `);
  const saveMinutes = (minutes: number) =>
    save(
      () =>
        new Promise<void>((resolve, reject) =>
          commit({
            variables: { minutesBefore: minutes },
            updater: (store) => {
              const settings = store.getRootField('updateWatchReminderSettings');
              if (settings !== null) {
                store.getRoot().setLinkedRecord(settings, 'watchReminderSettings');
              }
            },
            onCompleted: (_response, errors) =>
              errors != null && errors.length > 0
                ? reject(
                    Object.assign(new Error(errors[0]?.message ?? 'GraphQL error'), {
                      source: { errors },
                    }),
                  )
                : resolve(),
            onError: reject,
          }),
        ),
    );

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Text fw={600}>{t('notifications.watchReminder')}</Text>
        <AutoSaveNumberInput
          label={t('notifications.watchReminderMinutes')}
          description={t('notifications.watchReminderHint')}
          value={minutesBefore}
          onSave={saveMinutes}
          min={0}
          max={maxMinutes}
          allowDecimal={false}
          allowNegative={false}
          suffix={` ${t('notifications.watchReminderUnit')}`}
          w={240}
        />
      </Stack>
    </Card>
  );
}
