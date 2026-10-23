import { Button, Text } from '@mantine/core';
import { PictureInPicture2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import classes from './PipPlaceholder.module.css';

// PipPlaceholder は、PiP で再生している間、元のプレイヤーの場所に出す枠。
// プレイヤーと同じ大きさにして、PiP を始めても終えても、下の内容の位置が動かないようにする。
export function PipPlaceholder({ onBack }: { onBack: () => void }) {
  const { t } = useTranslation();

  return (
    <div className={classes.placeholder}>
      <PictureInPicture2 size={40} strokeWidth={1.5} aria-hidden />
      <Text size="sm">{t('player.pipActive')}</Text>
      <Button variant="default" size="xs" onClick={onBack}>
        {t('player.pipBack')}
      </Button>
    </div>
  );
}
