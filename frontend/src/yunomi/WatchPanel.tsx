import { Box, Paper, SegmentedControl, Stack, Text } from '@mantine/core';
import { useState, type ReactNode } from 'react';

// WatchPanelTab は、右の列の枠で切り替える中身の 1 つ。
export type WatchPanelTab = {
  value: string;
  label: string;
  content: ReactNode;
  // footer は、枠の下端に付けるもの (ライブ視聴の実況の投稿の欄)。
  footer?: ReactNode;
};

// WatchPanel は、視聴の画面の右の列に置く枠 (設計書 7.4)。実況のコメントの一覧と字幕の履歴を、見出しで切り替えて出す。
// tabs の先頭を既定にする。選んだ中身がなくなったら (実況を切ったときなど)、先頭に戻る。
export function WatchPanel({ tabs, label }: { tabs: readonly WatchPanelTab[]; label: string }) {
  // picked は、利用者が選んだ中身。選ぶまでは、先頭に従う。録画の実況の過去ログのように、後から先頭に足されたものに移れるようにする。
  const [picked, setPicked] = useState<string | null>(null);
  const current = tabs.find((tab) => tab.value === picked) ?? tabs[0];
  if (current === undefined) {
    return null;
  }

  return (
    <Paper withBorder radius="md">
      <Stack gap={0}>
        <Box px="sm" py="xs">
          {tabs.length > 1 ? (
            <SegmentedControl
              size="xs"
              value={current.value}
              onChange={setPicked}
              aria-label={label}
              data={tabs.map((tab) => ({ value: tab.value, label: tab.label }))}
            />
          ) : (
            <Text size="sm" fw={600}>
              {current.label}
            </Text>
          )}
        </Box>
        {current.content}
        {current.footer !== undefined && (
          <Box p="xs" style={{ borderTop: '1px solid var(--mantine-color-default-border)' }}>
            {current.footer}
          </Box>
        )}
      </Stack>
    </Paper>
  );
}
