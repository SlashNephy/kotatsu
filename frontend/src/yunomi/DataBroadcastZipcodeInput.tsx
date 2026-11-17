import { Badge, Group } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { AutoSaveTextInput } from '../settings/AutoSaveFields';
import { useDeviceSave } from '../settings/useAutoSave';
import { normalizeZipcode, useDataBroadcastZipcode } from './dataBroadcastZipcode';

// DataBroadcastZipcodeInput は、データ放送の受信機の郵便番号の入力欄 (#442)。この端末だけに覚え、
// 入力欄を離れたときか Enter で保存し、保存の通知を出す (#1740)。読めない値は、欄の下に理由を出して保存しない。
export function DataBroadcastZipcodeInput() {
  const { t } = useTranslation();
  const [zipcode, setZipcode] = useDataBroadcastZipcode();
  const deviceSave = useDeviceSave();

  return (
    <AutoSaveTextInput
      label={
        <Group gap={6} component="span">
          {t('settings.account.dataBroadcastZipcode')}
          <Badge size="sm" variant="light" color="gray" tt="none" component="span">
            {t('settings.account.deviceOnly')}
          </Badge>
        </Group>
      }
      description={t('settings.account.dataBroadcastZipcodeHelp')}
      placeholder="1000001"
      inputMode="numeric"
      autoComplete="postal-code"
      value={zipcode}
      validate={(value) =>
        normalizeZipcode(value) === null ? t('settings.account.dataBroadcastZipcodeInvalid') : null
      }
      normalize={(value) => normalizeZipcode(value) ?? value}
      onSave={(value) => {
        deviceSave(setZipcode)(value);

        return Promise.resolve(true);
      }}
      styles={{ input: { maxWidth: 280 } }}
    />
  );
}
