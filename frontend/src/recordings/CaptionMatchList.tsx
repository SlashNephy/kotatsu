import { Anchor, Badge, Button, Group, Stack, Text } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { HighlightedText, toHalfWidth, type HighlightKeyword } from '../futon';
import { formatDuration, type Gap } from '../yunomi/position';
import { captionStartSeconds } from './captionMatches';
import type { RecordingListSearch } from './listSearch';

// CaptionMatch は、字幕の文で探したときに当たった字幕の 1 つ (tenban の RecordingCaptionMatch)。
export type CaptionMatch = {
  readonly source: string;
  readonly from: string;
  readonly text: string;
};

// collapsedMatches は、たたんでいる間に出す、当たった字幕の数。
const collapsedMatches = 3;

// CaptionMatchList は、録画の検索で当たった字幕の文と位置を並べる (#2215)。押すと、その場面から再生する。
// 録画のカードは全体が詳細へのリンクなので、リンクが入れ子にならないよう、カードの外に置く。
export function CaptionMatchList({
  recordingId,
  recordedFrom,
  gaps,
  matches,
  keywords,
  listSearch,
}: {
  recordingId: string;
  recordedFrom: string;
  gaps: readonly Gap[];
  matches: readonly CaptionMatch[];
  keywords: readonly HighlightKeyword[];
  listSearch: RecordingListSearch;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  if (matches.length === 0) {
    return null;
  }
  const shown = expanded ? matches : matches.slice(0, collapsedMatches);
  const hidden = matches.length - shown.length;

  return (
    <Stack gap={4} mt={6} px={4}>
      {shown.map((match) => {
        const seconds = captionStartSeconds(recordedFrom, gaps, match.from);
        const text = toHalfWidth(match.text);
        const position = formatDuration(seconds);

        return (
          <Anchor
            key={`${match.source}:${match.from}`}
            renderRoot={(props) => (
              <Link
                {...props}
                to="/play/recording/$recordingId"
                params={{ recordingId }}
                search={{ t: seconds }}
                // 再生の画面の「戻る」でも、同じ絞り込みの一覧に戻す (#2146)。
                state={{ recordingsSearch: listSearch }}
              />
            )}
            underline="never"
            c="inherit"
            aria-label={t('recordings.captionMatchPlay', { position, text })}
          >
            <Group gap={6} wrap="nowrap" align="flex-start">
              <Text size="xs" c="blue" ff="monospace" style={{ flexShrink: 0 }}>
                {position}
              </Text>
              <Badge
                size="xs"
                variant="light"
                color="gray"
                radius="sm"
                tt="none"
                style={{ flexShrink: 0 }}
              >
                {t(
                  match.source === 'ARIB'
                    ? 'recordings.captionSource.ARIB'
                    : 'recordings.captionSource.AUTO',
                )}
              </Badge>
              <Text size="xs" lineClamp={2} style={{ whiteSpace: 'pre-line' }}>
                <HighlightedText text={text} keywords={keywords} />
              </Text>
            </Group>
          </Anchor>
        );
      })}
      {matches.length > collapsedMatches && (
        <Group>
          <Button size="compact-xs" variant="subtle" onClick={() => setExpanded(!expanded)}>
            {expanded
              ? t('recordings.captionMatchesLess')
              : t('recordings.captionMatchesMore', { count: hidden })}
          </Button>
        </Group>
      )}
    </Stack>
  );
}
