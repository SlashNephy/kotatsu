import { ActionIcon, Button, Group, ScrollArea, Stack, Text, UnstyledButton } from '@mantine/core';
import { useClipboard } from '@mantine/hooks';
import { Check, Copy, Subtitles } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { CaptionEntry } from './captionHistory';

// followPause は、利用者が履歴をスクロールしてから、新しい字幕の行へ追うのを止めておく時間 (ミリ秒)。読み返している間は動かさない。
const followPause = 10_000;

type Props = {
  entries: readonly CaptionEntry[];
  // label は、行の先頭に添える時刻を作る。
  label: (entry: CaptionEntry) => string;
  // onSelect は、行を押したときに呼ぶ。録画では、その字幕の位置へ移る。渡さなければ、行は押せない。
  onSelect?: (entry: CaptionEntry) => void;
};

// CaptionHistory は、字幕の履歴 (#450、設計書 7.4)。出た字幕を時刻の順に並べ、新しい字幕が出たらその行へ追う。
// 各行から字幕の文をコピーでき、録画では行を押すとその位置へ移る。
export function CaptionHistory({ entries, label, onSelect }: Props) {
  const { t } = useTranslation();
  const clipboard = useClipboard();
  const [copiedKey, setCopiedKey] = useState<number | null>(null);
  const viewport = useRef<HTMLDivElement | null>(null);
  // scrolledAt は、利用者が最後に履歴をスクロールした時刻。
  const scrolledAt = useRef(0);
  // newest は、最後に足した行。番号は足した順に増える。録画では、シークで戻ると途中に差し込むので、末尾の行とは限らない。
  const newest = entries.reduce<number | null>(
    (max, entry) => (max === null || entry.key > max ? entry.key : max),
    null,
  );
  useEffect(() => {
    const element = viewport.current;
    if (element === null || newest === null || Date.now() - scrolledAt.current < followPause) {
      return;
    }
    const row = element.querySelector<HTMLElement>(`[data-key="${newest}"]`);
    if (row === null) {
      return;
    }
    // 行が見える範囲に入るよう、履歴の中だけを動かす。scrollIntoView は、ページごと動かすので使わない。
    const view = element.getBoundingClientRect();
    const rect = row.getBoundingClientRect();
    if (rect.top < view.top) {
      element.scrollTop += rect.top - view.top;
    } else if (rect.bottom > view.bottom) {
      element.scrollTop += rect.bottom - view.bottom;
    }
  }, [newest]);
  const markScrolled = () => {
    scrolledAt.current = Date.now();
  };

  return (
    <ScrollArea
      h={240}
      type="auto"
      viewportRef={viewport}
      onWheel={markScrolled}
      onTouchMove={markScrolled}
      onPointerDown={markScrolled}
      aria-label={t('captionHistory.list')}
    >
      {entries.length === 0 ? (
        <Text size="sm" c="dimmed" p="xs">
          {t('captionHistory.empty')}
        </Text>
      ) : (
        <ul style={{ listStyle: 'none', margin: 0, padding: 0 }}>
          {entries.map((entry) => {
            const body = (
              <Group gap="xs" wrap="nowrap" align="flex-start">
                <Text size="xs" c="dimmed" ff="monospace" pt={2} style={{ flexShrink: 0 }}>
                  {label(entry)}
                </Text>
                <Text
                  size="sm"
                  style={{ flex: 1, whiteSpace: 'pre-line', overflowWrap: 'anywhere' }}
                >
                  {entry.text}
                </Text>
              </Group>
            );

            return (
              <li key={entry.key} data-key={entry.key}>
                <Group gap="xs" wrap="nowrap" px="xs" py={2}>
                  {onSelect === undefined ? (
                    <div style={{ flex: 1 }}>{body}</div>
                  ) : (
                    <UnstyledButton
                      style={{ flex: 1 }}
                      // 読み上げでは、時刻と字幕の文を名前にする。押すと移ることは、title で添える。
                      title={t('captionHistory.seek')}
                      onClick={() => onSelect(entry)}
                    >
                      {body}
                    </UnstyledButton>
                  )}
                  <ActionIcon
                    size="sm"
                    variant="subtle"
                    color="gray"
                    aria-label={t('captionHistory.copy')}
                    onClick={() => {
                      clipboard.copy(entry.text);
                      setCopiedKey(entry.key);
                    }}
                  >
                    {clipboard.copied && copiedKey === entry.key ? (
                      <Check size={14} aria-hidden />
                    ) : (
                      <Copy size={14} aria-hidden />
                    )}
                  </ActionIcon>
                </Group>
              </li>
            );
          })}
        </ul>
      )}
    </ScrollArea>
  );
}

// CaptionHistoryToggle は、字幕の履歴を開閉するボタンと、履歴。場所を取るので、既定では閉じる。
export function CaptionHistoryToggle(props: Props) {
  const { t } = useTranslation();
  const [opened, setOpened] = useState(false);

  return (
    <Stack gap="xs">
      <Group>
        <Button
          size="compact-sm"
          variant="default"
          leftSection={<Subtitles size={14} aria-hidden />}
          aria-expanded={opened}
          onClick={() => setOpened(!opened)}
        >
          {opened ? t('captionHistory.hide') : t('captionHistory.show')}
        </Button>
      </Group>
      {opened && <CaptionHistory {...props} />}
    </Stack>
  );
}
