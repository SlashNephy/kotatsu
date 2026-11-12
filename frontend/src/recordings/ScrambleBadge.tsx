import { Badge, type MantineSize } from '@mantine/core';
import { useTranslation } from 'react-i18next';

// ScrambleBadge は、スクランブルされたまま録れた録画の警告のバッジ (設計書 5.5、#1517)。状態とは別の印なので、状態のバッジと並べて出す。
// 警告なので、琥珀色にする (設計書 10.2)。
export function ScrambleBadge({ size }: { size?: MantineSize }) {
  const { t } = useTranslation();

  return (
    <Badge size={size} variant="light" color="amber">
      {t('recordings.scrambled')}
    </Badge>
  );
}
