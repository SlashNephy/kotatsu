import { Text } from '@mantine/core';
import { useTranslation } from 'react-i18next';

import { ProgramTitle, toHalfWidth } from '../futon';
import classes from './PlayerControls.module.css';
import { zonedFormat } from '../futon';

const timeFormat = zonedFormat({
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

type Program = {
  readonly name: string;
  readonly startAt: string;
  readonly endAt: string;
  readonly endUndecided: boolean;
};

type Props = {
  // visible は、操作バーと一緒に出すかどうか。
  visible: boolean;
  serviceName: string;
  current: Program | null;
  next: Program | null;
};

// ProgramOverlay は、ライブ視聴で、操作バーを出している間、映像の上端に、放送中の番組と次の番組を重ねる (#438)。
// ページでは番組名を映像の下に出すので、映像の下が見えない別のウィンドウ (Document Picture-in-Picture) でだけ使う。
export function ProgramOverlay({ visible, serviceName, current, next }: Props) {
  const { t } = useTranslation();
  const range = (program: Program) =>
    t('range', {
      from: timeFormat.format(Date.parse(program.startAt)),
      to: program.endUndecided
        ? t('live.overlay.endUndecided')
        : timeFormat.format(Date.parse(program.endAt)),
    });

  return (
    <div className={classes.programInfo} data-visible={visible} aria-hidden={!visible}>
      <Text size="sm" fw={700} lineClamp={1}>
        {current === null ? toHalfWidth(serviceName) : <ProgramTitle name={current.name} />}
      </Text>
      <Text size="xs">
        {current === null ? t('live.noProgram') : `${toHalfWidth(serviceName)}・${range(current)}`}
      </Text>
      {next !== null && (
        <Text size="xs" lineClamp={1} c="gray.3">
          {t('live.overlay.next', { range: range(next) })} <ProgramTitle name={next.name} />
        </Text>
      )}
    </div>
  );
}
