import {
  Center,
  SegmentedControl,
  Tooltip,
  VisuallyHidden,
  useMantineColorScheme,
  type MantineColorScheme,
} from '@mantine/core';
import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

const schemes: MantineColorScheme[] = ['auto', 'light', 'dark'];

const icons: Record<MantineColorScheme, LucideIcon> = {
  auto: Monitor,
  light: Sun,
  dark: Moon,
};

function isColorScheme(value: string): value is MantineColorScheme {
  return (schemes as string[]).includes(value);
}

// ColorSchemeToggle は、ライトテーマとダークテーマを切り替える。auto は OS の設定に従う。
// 場所を取らないようにアイコンだけを並べ、名前はツールチップと読み上げで補う。
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
      data={schemes.map((scheme) => {
        const Icon = icons[scheme];
        const label = t(`colorScheme.${scheme}`);
        return {
          value: scheme,
          label: (
            <Tooltip label={label} withinPortal>
              <Center>
                <Icon size={16} aria-hidden />
                <VisuallyHidden>{label}</VisuallyHidden>
              </Center>
            </Tooltip>
          ),
        };
      })}
    />
  );
}
