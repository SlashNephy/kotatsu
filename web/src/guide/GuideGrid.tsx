import { Box, Text, UnstyledButton } from '@mantine/core';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useEffect, useMemo, useRef, useState } from 'react';

import { genreBackground } from './genres';
import {
  gridHeight,
  layoutColumn,
  minuteHeight,
  programBox,
  timeToY,
  visibleRange,
  yToTime,
  type ColumnLayout,
  type GuideWindow,
} from './layout';

// GuideProgram は、番組表の枠に出す番組。
export type GuideProgram = {
  id: string;
  startAt: number;
  endAt: number;
  endUndecided: boolean;
  name: string;
  genre: number | undefined;
};

// GuideColumn は、番組表の 1 列 (1 つのサービス)。programs は開始時刻の順に並べる。
export type GuideColumn = {
  id: string;
  name: string;
  programs: GuideProgram[];
};

type Props = {
  window: GuideWindow;
  columns: GuideColumn[];
  // now は、いまの時刻。時間帯に入っていれば、線を引く。
  now: number;
  // initialTime は、最初に表示する時刻。
  initialTime: number;
  onSelect: (programId: string) => void;
};

const columnWidth = 168;
const axisWidth = 44;
const headerHeight = 36;
// overscanPixels は、見えている範囲の上下に、余分に描く高さ。速くスクロールしても、空白が見えにくくする。
const overscanPixels = 600;

const timeFormat = new Intl.DateTimeFormat('ja-JP', {
  hour: '2-digit',
  minute: '2-digit',
  timeZone: 'Asia/Tokyo',
});

// GuideGrid は、横にサービス、縦に時刻を並べた番組表。
// サービスの数と番組の数が多くても滑らかに動くよう、横はサービスの列を、縦は列の中の番組を、見えている範囲だけ描く。
export function GuideGrid({ window, columns, now, initialTime, onSelect }: Props) {
  const scroller = useRef<HTMLDivElement>(null);
  const [viewport, setViewport] = useState({ top: 0, height: 800 });

  const virtualizer = useVirtualizer({
    horizontal: true,
    count: columns.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => columnWidth,
    overscan: 2,
  });

  // 縦のスクロールの位置を、描く番組を決めるために追う。描画は、フレームごとに 1 回にまとめる。
  useEffect(() => {
    const element = scroller.current;
    if (element === null) {
      return;
    }
    let frame = 0;
    const update = () => {
      frame = 0;
      setViewport({ top: element.scrollTop, height: element.clientHeight });
    };
    const onScroll = () => {
      if (frame === 0) {
        frame = requestAnimationFrame(update);
      }
    };
    update();
    element.addEventListener('scroll', onScroll, { passive: true });
    const observer = new ResizeObserver(onScroll);
    observer.observe(element);

    return () => {
      element.removeEventListener('scroll', onScroll);
      observer.disconnect();
      cancelAnimationFrame(frame);
    };
  }, []);

  // 最初と、日付を変えたときに、指定の時刻が上の方に来るようにスクロールする。
  useEffect(() => {
    const element = scroller.current;
    if (element !== null) {
      element.scrollTop = Math.max(timeToY(window, initialTime) - 60 * minuteHeight * 0.5, 0);
    }
  }, [window, initialTime]);

  const visibleFrom = yToTime(window, viewport.top - overscanPixels);
  const visibleTo = yToTime(window, viewport.top + viewport.height + overscanPixels);
  const height = gridHeight(window);
  const hours = useMemo(() => {
    const list: number[] = [];
    for (let t = window.from; t < window.to; t += 3_600_000) {
      list.push(t);
    }

    return list;
  }, [window]);
  // 列の番組の配置は、番組が変わったときだけ計算し直す。
  const layouts = useMemo(() => columns.map((column) => layoutColumn(column.programs)), [columns]);
  const totalWidth = axisWidth + virtualizer.getTotalSize();
  const nowY = now >= window.from && now < window.to ? timeToY(window, now) : undefined;

  return (
    <Box
      ref={scroller}
      data-testid="guide-scroller"
      style={{
        overflow: 'auto',
        height: 'calc(100dvh - 180px)',
        minHeight: 320,
        border: '1px solid var(--mantine-color-default-border)',
        borderRadius: 'var(--mantine-radius-md)',
        overscrollBehavior: 'contain',
      }}
    >
      <Box
        style={{
          position: 'sticky',
          top: 0,
          zIndex: 3,
          width: totalWidth,
          height: headerHeight,
          background: 'var(--mantine-color-body)',
          borderBottom: '1px solid var(--mantine-color-default-border)',
        }}
      >
        <Box
          style={{
            position: 'sticky',
            left: 0,
            zIndex: 4,
            width: axisWidth,
            height: headerHeight,
            background: 'var(--mantine-color-body)',
          }}
        />
        {virtualizer.getVirtualItems().map((item) => (
          <Text
            key={item.key}
            size="sm"
            fw={700}
            truncate
            title={columns[item.index]!.name}
            style={{
              position: 'absolute',
              top: 0,
              left: axisWidth + item.start,
              width: item.size,
              height: headerHeight,
              lineHeight: `${headerHeight}px`,
              paddingInline: 6,
              borderLeft: '1px solid var(--mantine-color-default-border)',
            }}
          >
            {columns[item.index]!.name}
          </Text>
        ))}
      </Box>
      <Box style={{ position: 'relative', width: totalWidth, height }}>
        <Box
          style={{
            position: 'sticky',
            left: 0,
            zIndex: 2,
            width: axisWidth,
            height,
            background: 'var(--mantine-color-body)',
            borderRight: '1px solid var(--mantine-color-default-border)',
          }}
        >
          {hours.map((hour) => (
            <Text
              key={hour}
              size="xs"
              c="dimmed"
              ta="center"
              style={{ position: 'absolute', top: timeToY(window, hour), width: axisWidth }}
            >
              {timeFormat.format(hour).slice(0, 2)}
            </Text>
          ))}
        </Box>
        {virtualizer.getVirtualItems().map((item) => (
          <GuideColumnView
            key={item.key}
            window={window}
            column={columns[item.index]!}
            layout={layouts[item.index]!}
            left={axisWidth + item.start}
            width={item.size}
            visibleFrom={visibleFrom}
            visibleTo={visibleTo}
            onSelect={onSelect}
          />
        ))}
        {nowY !== undefined && (
          <Box
            aria-hidden
            style={{
              position: 'absolute',
              top: nowY,
              left: axisWidth,
              width: totalWidth - axisWidth,
              height: 2,
              zIndex: 1,
              background: 'var(--mantine-color-red-6)',
              pointerEvents: 'none',
            }}
          />
        )}
      </Box>
    </Box>
  );
}

type ColumnProps = {
  window: GuideWindow;
  column: GuideColumn;
  layout: ColumnLayout;
  left: number;
  width: number;
  visibleFrom: number;
  visibleTo: number;
  onSelect: (programId: string) => void;
};

function GuideColumnView({
  window,
  column,
  layout,
  left,
  width,
  visibleFrom,
  visibleTo,
  onSelect,
}: ColumnProps) {
  const { start, end } = visibleRange(column.programs, layout, visibleFrom, visibleTo);

  return (
    <>
      {column.programs.slice(start, end).map((program, offset) => {
        const index = start + offset;
        const box = programBox(window, program, layout.ends[index]!);
        // 重なる番組は、列の幅を分けて横に並べる。
        const laneWidth = width / layout.laneCounts[index]!;

        return (
          <UnstyledButton
            key={program.id}
            data-program-id={program.id}
            onClick={() => onSelect(program.id)}
            title={program.name}
            style={{
              position: 'absolute',
              top: box.top,
              left: left + laneWidth * layout.lanes[index]!,
              width: laneWidth,
              height: box.height,
              overflow: 'hidden',
              // ボタンは、中身を縦の中央に寄せるので、上に詰める。
              display: 'flex',
              alignItems: 'flex-start',
              padding: '2px 6px',
              background: genreBackground(program.genre),
              borderLeft: '1px solid var(--mantine-color-default-border)',
              borderBottom: '1px solid var(--mantine-color-default-border)',
              fontSize: 'var(--mantine-font-size-xs)',
              lineHeight: 1.35,
            }}
          >
            <span>
              <Text span size="xs" fw={700} mr={4}>
                {timeFormat.format(program.startAt)}
              </Text>
              {program.name}
            </span>
          </UnstyledButton>
        );
      })}
    </>
  );
}
