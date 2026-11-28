import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { CaptionHistory } from './CaptionHistory';
import { insertRecordedCaption, normalizeCaption, type CaptionEntry } from './captionHistory';
import { useDRCSReplacements } from './useDRCSReplacements';
import { watchCommentsHeight } from './WatchLayout';

// 録画、タイムシフト、ライブラリのファイルの再生で、字幕の履歴 (#450) を扱う機能の部品 (#2244)。字幕が出たときの再生位置で並べ、
// 行を押すとその位置へ移る。今出ている字幕の文は、キャプチャに書き込む (#458)。

// RecordedCaptions は、useRecordedCaptions が返す、字幕の履歴の口。
export type RecordedCaptions = {
  // onCaptionText は、プレイヤーが字幕の文を出したときに呼ぶ。
  onCaptionText: (text: string) => void;
  // currentCaption は、今出ている字幕の文。出ていなければ null。
  currentCaption: () => string | null;
  // tab は、右の欄に出す字幕の履歴のタブ。label は、行に添える時刻、onSelect は、行を押したときの移り先。
  tab: (
    label: (entry: CaptionEntry) => string,
    onSelect: (entry: CaptionEntry) => void,
  ) => { value: string; label: string; content: React.ReactNode };
};

// useRecordedCaptions は、字幕の履歴を用意する。currentPosition は、字幕が出たときの再生位置を読む。
export function useRecordedCaptions(currentPosition: () => number): RecordedCaptions {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<readonly CaptionEntry[]>([]);
  const current = useRef<string | null>(null);
  // 字幕の外字を置き換えるテーブルを読む (#825)。
  const drcs = useDRCSReplacements();

  return {
    onCaptionText: (text) => {
      const normalized = normalizeCaption(text);
      current.current = normalized === '' ? null : normalized;
      const at = currentPosition();
      setEntries((list) => insertRecordedCaption(list, at, text));
    },
    currentCaption: () => current.current,
    tab: (label, onSelect) => ({
      value: 'captions',
      label: t('captionHistory.tab'),
      content: (
        <CaptionHistory
          entries={entries}
          label={label}
          onSelect={onSelect}
          canRegisterDRCS={drcs.canRegister}
          height={watchCommentsHeight}
        />
      ),
    }),
  };
}
