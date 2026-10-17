import { Alert, Card, NumberInput, Stack, Text } from '@mantine/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useMutation } from 'react-relay';

import type { WatchReminderSettingsMutation } from './__generated__/WatchReminderSettingsMutation.graphql';

// maxMinutes は、視聴予約の通知を、放送の何分前に送るかの上限 (設計書 4.8、付録 A)。
const maxMinutes = 60;

// WatchReminderSettings は、視聴予約の通知を、放送の何分前に送るかの設定 (設計書 4.8)。入力欄を離れたときに保存する。
export function WatchReminderSettings({ minutesBefore }: { minutesBefore: number }) {
  const { t } = useTranslation();
  const [value, setValue] = useState<number | string>(minutesBefore);
  const [error, setError] = useState(false);
  // ほかの端末で変えて読み直したら、入力欄にも反映する。
  useEffect(() => setValue(minutesBefore), [minutesBefore]);
  const [commit, inFlight] = useMutation<WatchReminderSettingsMutation>(graphql`
    mutation WatchReminderSettingsMutation($minutesBefore: Int!) {
      updateWatchReminderSettings(minutesBefore: $minutesBefore) {
        minutesBefore
      }
    }
  `);
  const save = () => {
    const minutes = typeof value === 'number' ? value : Number.NaN;
    if (minutes === minutesBefore) {
      return;
    }
    if (!Number.isInteger(minutes) || minutes < 0 || minutes > maxMinutes) {
      setError(true);

      return;
    }
    setError(false);
    commit({
      variables: { minutesBefore: minutes },
      updater: (store) => {
        const settings = store.getRootField('updateWatchReminderSettings');
        if (settings !== null) {
          store.getRoot().setLinkedRecord(settings, 'watchReminderSettings');
        }
      },
      onCompleted: (_response, errors) => setError(errors != null && errors.length > 0),
      onError: () => setError(true),
    });
  };

  return (
    <Card withBorder>
      <Stack gap="xs">
        <Text fw={600}>{t('notifications.watchReminder')}</Text>
        <NumberInput
          label={t('notifications.watchReminderMinutes')}
          description={t('notifications.watchReminderHint')}
          value={value}
          onChange={setValue}
          onBlur={save}
          min={0}
          max={maxMinutes}
          allowDecimal={false}
          allowNegative={false}
          suffix={` ${t('notifications.watchReminderUnit')}`}
          disabled={inFlight}
          w={240}
        />
        {error && (
          <Alert color="red" role="alert">
            {t('notifications.watchReminderError')}
          </Alert>
        )}
      </Stack>
    </Card>
  );
}
