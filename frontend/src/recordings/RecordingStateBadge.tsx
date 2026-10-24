import { Badge, type MantineSize } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import classes from './RecordingStateBadge.module.css';
import { stateColor, stateKey } from './states';

// RecordingStateBadge は、録画の状態のバッジ。録画中は、赤い点を明滅させて、録っている最中だと分かるようにする。
export function RecordingStateBadge({ state, size }: { state: string; size?: MantineSize }) {
  const { t } = useTranslation();

  return (
    <Badge
      size={size}
      variant="light"
      color={stateColor(state)}
      leftSection={state === 'RECORDING' ? <span className={classes.dot} aria-hidden /> : undefined}
    >
      {t(stateKey(state))}
    </Badge>
  );
}
