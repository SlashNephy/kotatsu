// ホームとライブ視聴の一覧で共有する、放送中の番組のタブ (設計書 10.3)。

import { useEffect, useState } from 'react';

import { rankByJikkyoForce } from './jikkyoRanking';
import { sharesWithListed } from './sharedEvent';

export type NowPlayingRow = {
  service: {
    id: string;
    name: string;
    logoUrl?: string | null;
    channelType: { type: string };
    jikkyo?: { id: string; force?: number | null } | null;
  };
  programs: readonly {
    name: string;
    startAt: string;
    endAt: string;
    sharedFrom?: { id: string } | null;
  }[];
};

export type NowPlayingTab = {
  value: string;
  label: string;
  rows: readonly NowPlayingRow[];
};

export type NowPlayingTabsState = {
  tabs: readonly NowPlayingTab[];
  value: string;
  onChange: (value: string) => void;
  rows: readonly NowPlayingRow[];
};

// pinnedTab と forceTab は、特別なタブの値。チャンネル種別とぶつからないよう、小文字にする。
const pinnedTab = 'pinned';
const forceTab = 'force';

// nowPlayingTabs は、タブと、それぞれに出す局を返す。局のないタブは作らない。並びは、ピン留め、人気の実況、チャンネル種別の順。
// ピン留めのタブには、番組表にあるピン留めの局を、ピン留めした順に出す。イベント共有のサブチャンネルでも省かない。
// チャンネル種別のタブでは、メインチャンネルと同じ番組を流しているサブチャンネルを省く。
export function nowPlayingTabs(
  channelTypes: readonly { type: string; displayName: string }[],
  rows: readonly NowPlayingRow[],
  pinned: readonly string[],
  labels: { pinned: string; force: string },
): NowPlayingTab[] {
  const listed = new Set(rows.map((row) => row.service.id));
  const candidates = [
    {
      value: pinnedTab,
      label: labels.pinned,
      rows: pinned.flatMap((id) => rows.filter((row) => row.service.id === id)),
    },
    { value: forceTab, label: labels.force, rows: rankByJikkyoForce(rows) },
    ...channelTypes.map((channelType) => ({
      value: channelType.type,
      label: channelType.displayName,
      rows: rows.filter(
        (row) =>
          row.service.channelType.type === channelType.type &&
          !sharesWithListed(row.programs[0], listed),
      ),
    })),
  ];

  return candidates.filter((tab) => tab.rows.length > 0);
}

function isChannelType(tab: NowPlayingTab): boolean {
  return tab.value !== pinnedTab && tab.value !== forceTab;
}

// useNowPlayingTabs は、選んでいるタブを覚える。開いたときは、ピン留めのタブがあればそれを、なければ最初のチャンネル種別を選ぶ。
// 選んでいるタブがなくなったら、最初のチャンネル種別に戻す。後でまたタブができても、そのタブへは戻さない。
export function useNowPlayingTabs(tabs: readonly NowPlayingTab[]): NowPlayingTabsState {
  const [selected, setSelected] = useState(
    () => tabs.find((tab) => tab.value !== forceTab)?.value ?? '',
  );
  const fallback = tabs.find(isChannelType)?.value ?? '';
  const gone = !tabs.some((tab) => tab.value === selected);
  useEffect(() => {
    if (gone) {
      setSelected(fallback);
    }
  }, [gone, fallback]);
  const value = gone ? fallback : selected;

  return {
    tabs,
    value,
    onChange: setSelected,
    rows: tabs.find((tab) => tab.value === value)?.rows ?? [],
  };
}
