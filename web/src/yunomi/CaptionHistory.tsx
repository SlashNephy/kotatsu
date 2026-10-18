import { ActionIcon, Button, Group, ScrollArea, Stack, Text, UnstyledButton } from '@mantine/core';
import { useClipboard } from '@mantine/hooks';
import { Check, Copy, Subtitles } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { CaptionEntry } from './captionHistory';

// stickThreshold は、履歴の末尾からこの画素の内にいれば、新しい字幕が出たときに末尾へ追う範囲。
const stickThreshold = 48;

type Props = {
  entries: readonly CaptionEntry[];
  // label は、行の先頭に添える時刻を作る。
  label: (entry: CaptionEntry) => string;
  // onSelect は、行を押したときに呼ぶ。録画では、その字幕の位置へ移る。渡さなければ、行は押せない。
  onSelect?: (entry: CaptionEntry) => void;
};

// CaptionHistory は、字幕の履歴 (#450、設計書 7.4)。出た字幕を古い順に並べ、新しいものを下に足す。
// 各行から字幕の文をコピーでき、録画では行を押すとその位置へ移る。
export function CaptionHistory({ entries, label, onSelect }: Props) {
  const { t } = useTranslation();
  const clipboard = useClipboard();
  const [copiedKey, setCopiedKey] = useState<number | null>(null);
  const viewport = useRef<HTMLDivElement | null>(null);
  const stick = useRef(true);
  const last = entries.at(-1)?.key;
  useEffect(() => {
    const element = viewport.current;
    if (element !== null && stick.current) {
      element.scrollTop = element.scrollHeight;
    }
  }, [last]);

  return (
    <ScrollArea
      h={240}
      type="auto"
      viewportRef={viewport}
      onScrollPositionChange={() => {
        const element = viewport.current;
        if (element !== null) {
          stick.current =
            element.scrollHeight - element.scrollTop - element.clientHeight < stickThreshold;
        }
      }}
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
              <li key={entry.key}>
                <Group gap="xs" wrap="nowrap" px="xs" py={2}>
                  {onSelect === undefined ? (
                    <div style={{ flex: 1 }}>{body}</div>
                  ) : (
                    <UnstyledButton
                      style={{ flex: 1 }}
                      aria-label={t('captionHistory.seek', { time: label(entry) })}
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
