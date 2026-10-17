import {
  Badge,
  Button,
  Card,
  ColorSwatch,
  Group,
  Select,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Switch,
  Text,
  Title,
} from '@mantine/core';
import { useTranslation } from 'react-i18next';

import {
  colorGenres,
  defaultGuideAppearance,
  genreColorChoices,
  useGuideAppearance,
  type Density,
  type GenreColor,
} from './appearance';
import { genreKey } from './genres';

const swatch = (color: string) => `var(--mantine-color-${color}-light)`;

// GuideAppearanceSettings は、番組表の見た目 (密度、通販番組のグレー、ジャンルの色) を、この端末で変える設定 (#483)。
export function GuideAppearanceSettings() {
  const { t } = useTranslation();
  const [appearance, setAppearance] = useGuideAppearance();
  const colorOptions = genreColorChoices.map((color) => ({
    value: color,
    label: t(`settings.guide.colors.${color}`),
  }));

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Group gap={6}>
          <Title order={3} size="h5">
            {t('settings.guide.title')}
          </Title>
          {/* アカウントの設定と並ぶので、この端末だけの設定であることを示す。 */}
          <Badge size="sm" variant="light" color="gray" tt="none">
            {t('settings.account.deviceOnly')}
          </Badge>
        </Group>
        <Stack gap={4}>
          <Text size="sm" fw={500}>
            {t('settings.guide.density')}
          </Text>
          <SegmentedControl
            aria-label={t('settings.guide.density')}
            value={appearance.density}
            onChange={(value) => setAppearance({ ...appearance, density: value as Density })}
            data={[
              { value: 'compact', label: t('settings.guide.densities.compact') },
              { value: 'normal', label: t('settings.guide.densities.normal') },
              { value: 'wide', label: t('settings.guide.densities.wide') },
            ]}
          />
        </Stack>
        <Switch
          label={t('settings.guide.grayShopping')}
          description={t('settings.guide.grayShoppingHelp')}
          checked={appearance.grayShopping}
          onChange={(event) =>
            setAppearance({ ...appearance, grayShopping: event.currentTarget.checked })
          }
        />
        <Stack gap={4}>
          <Text size="sm" fw={500}>
            {t('settings.guide.genreColors')}
          </Text>
          <SimpleGrid cols={{ base: 1, xs: 2, md: 3 }} spacing="xs" verticalSpacing="xs">
            {colorGenres.map((genre) => {
              const color = appearance.genreColors[genre] ?? 'gray';

              return (
                <Select
                  key={genre}
                  size="xs"
                  label={t(genreKey(genre))}
                  data={colorOptions}
                  value={color}
                  allowDeselect={false}
                  leftSection={<ColorSwatch color={swatch(color)} size={16} />}
                  renderOption={({ option }) => (
                    <Group gap="xs" wrap="nowrap">
                      <ColorSwatch color={swatch(option.value)} size={16} />
                      {option.label}
                    </Group>
                  )}
                  onChange={(value) => {
                    if (value !== null) {
                      setAppearance({
                        ...appearance,
                        genreColors: { ...appearance.genreColors, [genre]: value as GenreColor },
                      });
                    }
                  }}
                />
              );
            })}
          </SimpleGrid>
        </Stack>
        <Group justify="flex-end">
          <Button size="xs" variant="default" onClick={() => setAppearance(defaultGuideAppearance)}>
            {t('settings.guide.reset')}
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
