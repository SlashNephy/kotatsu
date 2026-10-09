import { SegmentedControl, useMantineColorScheme, type MantineColorScheme } from '@mantine/core';
import { useTranslation } from 'react-i18next';

const schemes: MantineColorScheme[] = ['auto', 'light', 'dark'];

function isColorScheme(value: string): value is MantineColorScheme {
  return (schemes as string[]).includes(value);
}

// ColorSchemeToggle は、ライトテーマとダークテーマを切り替える。auto は OS の設定に従う。
export function ColorSchemeToggle() {
  const { t } = useTranslation();
  const { colorScheme, setColorScheme } = useMantineColorScheme();

  return (
    <SegmentedControl
      aria-label={t('colorScheme.label')}
      value={colorScheme}
      onChange={(value) => {
        if (isColorScheme(value)) {
          setColorScheme(value);
        }
      }}
      data={schemes.map((scheme) => ({ value: scheme, label: t(`colorScheme.${scheme}`) }))}
    />
  );
}
