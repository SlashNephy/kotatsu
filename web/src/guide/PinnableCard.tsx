import type { ReactNode } from 'react';

import classes from './PinnableCard.module.css';
import { PinButton } from './PinButton';

type Props = {
  serviceId: string;
  serviceName: string;
  pinned: boolean;
  children: ReactNode;
};

// PinnableCard は、カードの右上にピン留めのボタンを重ねる (#485)。ホームの放送中の番組と、ライブの一覧で使う。
// カードはリンクなので、ボタンはカードの中ではなく上に重ねる。
export function PinnableCard({ serviceId, serviceName, pinned, children }: Props) {
  return (
    <div className={classes.root}>
      {children}
      <div className={classes.pin} data-pinned={pinned}>
        <PinButton serviceId={serviceId} serviceName={serviceName} pinned={pinned} size="xs" />
      </div>
    </div>
  );
}
