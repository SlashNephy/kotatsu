import { Badge, Group, TextInput } from '@mantine/core';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { normalizeZipcode, useDataBroadcastZipcode } from './dataBroadcastZipcode';

// DataBroadcastZipcodeInput は、データ放送の受信機の郵便番号の入力欄 (#442)。この端末だけに覚え、入力欄を離れたときに保存する。
export function DataBroadcastZipcodeInput() {
  const { t } = useTranslation();
  const [zipcode, setZipcode] = useDataBroadcastZipcode();
  const [value, setValue] = useState(zipcode);
  const [invalid, setInvalid] = useState(false);
  // ほかのタブで変えたら、入力欄にも反映する。
  useEffect(() => setValue(zipcode), [zipcode]);
  const save = () => {
    const normalized = normalizeZipcode(value);
    if (normalized === null) {
      setInvalid(true);

      return;
    }
    setInvalid(false);
    setValue(normalized);
    setZipcode(normalized);
  };

  return (
    <TextInput
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
      value={value}
      onChange={(event) => setValue(event.currentTarget.value)}
      onBlur={save}
      error={invalid ? t('settings.account.dataBroadcastZipcodeInvalid') : undefined}
      styles={{ input: { maxWidth: 280 } }}
    />
  );
}
