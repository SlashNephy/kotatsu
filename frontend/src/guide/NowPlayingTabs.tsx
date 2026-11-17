import { SegmentedControl } from '@mantine/core';
import type { CSSProperties } from 'react';

import type { NowPlayingTabsState } from './useNowPlayingTabs';
import classes from './NowPlayingTabs.module.css';

type Props = {
  state: NowPlayingTabsState;
  className?: string;
  style?: CSSProperties;
};

// NowPlayingTabs は、放送中の番組のタブ。選べるタブが 1 つだけなら出さない。見出しとの並べ方は、使う画面が決める。
// 狭い画面でタブが収まらなければ、ページではなくタブの行だけを横にスクロールさせる (#1879)。
export function NowPlayingTabs({ state, className, style }: Props) {
  if (state.tabs.length <= 1) {
    return null;
  }

  return (
    <div className={[classes.scroller, className].filter(Boolean).join(' ')} style={style}>
      <SegmentedControl
        size="xs"
        value={state.value}
        onChange={state.onChange}
        data={state.tabs.map((tab) => ({ value: tab.value, label: tab.label }))}
      />
    </div>
  );
}
