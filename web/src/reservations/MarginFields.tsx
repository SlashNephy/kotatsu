import { Group, Loader, NumberInput } from '@mantine/core';
import { Suspense } from 'react';
import { useTranslation } from 'react-i18next';
import { graphql, useLazyLoadQuery } from 'react-relay';

import type { MarginFieldsQuery } from './__generated__/MarginFieldsQuery.graphql';

// maxMarginSeconds は、マージンの上限 (internal/reserve の maxMarginSeconds)。
const maxMarginSeconds = 60 * 60;

type Props = {
  start: number | null;
  end: number | null;
  onChange: (margins: { start: number | null; end: number | null }) => void;
  disabled?: boolean;
  // rule は、ルール予約で、空のときに従うルールの名前と、そのルールのマージン。ルールが決めていない項目は、全体の既定値に従う。
  rule?: { name: string; start: number | null; end: number | null };
};

// MarginFields は、録画の開始前と終了後のマージン (秒) の入力欄。空なら全体の既定値に従うので、
// 既定値を入力欄の中と説明に示す。ルールの画面と予約の設定で使う。
// 既定値を読む間は、ここだけを待たせ、まわりのフォームの入力を失わないようにする。
export function MarginFields(props: Props) {
  return (
    <Suspense fallback={<Loader size="xs" />}>
      <Fields {...props} />
    </Suspense>
  );
}

function Fields({ start, end, onChange, disabled = false, rule }: Props) {
  const { t } = useTranslation();
  const data = useLazyLoadQuery<MarginFieldsQuery>(
    graphql`
      query MarginFieldsQuery {
        systemSettings {
          recordingDefaults {
            startMarginSeconds
            endMarginSeconds
          }
        }
      }
    `,
    {},
    { fetchPolicy: 'store-or-network' },
  );
  const defaults = data.systemSettings.recordingDefaults;
  const field = (
    label: string,
    value: number | null,
    global: number,
    fromRule: number | null | undefined,
    set: (value: number | null) => void,
  ) => {
    const fallback = fromRule ?? global;

    return (
      <NumberInput
        label={label}
        description={
          rule !== undefined && fromRule != null
            ? t('margins.descriptionRule', { name: rule.name, seconds: fallback })
            : t('margins.description', { seconds: fallback })
        }
        placeholder={t('margins.placeholder', { seconds: fallback })}
        maw={240}
        value={value ?? ''}
        onChange={(next) => set(typeof next === 'number' ? next : null)}
        min={0}
        max={maxMarginSeconds}
        allowDecimal={false}
        allowNegative={false}
        suffix={t('margins.seconds')}
        disabled={disabled}
      />
    );
  };

  return (
    <Group align="flex-start" gap="md">
      {field(t('margins.start'), start, defaults.startMarginSeconds, rule?.start, (value) =>
        onChange({ start: value, end }),
      )}
      {field(t('margins.end'), end, defaults.endMarginSeconds, rule?.end, (value) =>
        onChange({ start, end: value }),
      )}
    </Group>
  );
}
