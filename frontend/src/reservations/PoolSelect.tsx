import { Select } from '@mantine/core';
import { useTranslation } from 'react-i18next';

// inheritPool は、保存先プールを上書きしない選択肢の値。プールの ID とぶつからないよう、ID に使えない空白を含める。
const inheritPool = ' inherit';

type Props = {
  // pools は、選べる保存先プールの ID。
  pools: readonly string[];
  // value は、上書きする保存先プール。null なら上書きしない。
  value: string | null;
  onChange: (value: string | null) => void;
  // inheritedLabel は、上書きしないときに従う設定の説明 (「自動で選ぶ」、「ルール「…」の設定に従う」など)。
  inheritedLabel: string;
  disabled?: boolean;
};

// PoolSelect は、予約とルールで、保存先プールを選ぶ欄 (設計書 5.6、#463、#464)。
export function PoolSelect({ pools, value, onChange, inheritedLabel, disabled }: Props) {
  const { t } = useTranslation();
  // 選んであったプールを、設定から外していても、選んだまま残して見せる。
  const ids = value === null || pools.includes(value) ? pools : [...pools, value];

  return (
    <Select
      label={t('admin.settings.pool')}
      data={[
        { value: inheritPool, label: inheritedLabel },
        ...ids.map((id) => ({ value: id, label: id })),
      ]}
      value={value ?? inheritPool}
      onChange={(next) => onChange(next === null || next === inheritPool ? null : next)}
      allowDeselect={false}
      disabled={disabled}
    />
  );
}
