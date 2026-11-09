import { SegmentedControl } from '@mantine/core';
import type { CSSProperties } from 'react';

import type { NowPlayingTabsState } from './useNowPlayingTabs';

type Props = {
  state: NowPlayingTabsState;
  className?: string;
  style?: CSSProperties;
};

// NowPlayingTabs は、放送中の番組のタブ。選べるタブが 1 つだけなら出さない。見出しとの並べ方は、使う画面が決める。
export function NowPlayingTabs({ state, className, style }: Props) {
  if (state.tabs.length <= 1) {
    return null;
  }

  return (
    <SegmentedControl
      className={className}
      style={style}
      size="xs"
      value={state.value}
      onChange={state.onChange}
      data={state.tabs.map((tab) => ({ value: tab.value, label: tab.label }))}
    />
  );
}
