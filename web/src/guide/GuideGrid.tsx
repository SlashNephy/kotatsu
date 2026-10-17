import { ActionIcon, Anchor, Box, Group, Text, Tooltip, UnstyledButton } from '@mantine/core';
import { Link } from '@tanstack/react-router';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { BellRing, CalendarRange } from 'lucide-react';

import { densities, genreBackground, useGuideAppearance } from './appearance';
import {
  broadcastDay,
  gridHeight,
  layoutColumn,
  stackedBoxes,
  minuteHeightOf,
  timeScale,
  timeToY,
  visibleRange,
  yToTime,
  type ColumnLayout,
  type GuideWindow,
  type Box as ProgramBox,
} from './layout';
import { guideHour, guideTime } from './hourNotation';
import { ServiceLabel } from './ServiceLabel';
import { useDragScroll } from './useDragScroll';
import { useTranslation } from 'react-i18next';

import { ProgramTitle, splitProgramMarks, toHalfWidth } from '../futon';

// GuideProgram は、番組表の枠に出す番組。
export type GuideProgram = {
  id: string;
  startAt: number;
  endAt: number;
  endUndecided: boolean;
  name: string;
  // genre は、番組の最初のジャンル。枠の色を決める。
  genre: GuideGenre | undefined;
  // shared は、サブチャンネルで、元のサービスと同じ番組を流していること。枠を薄くし、番組名を出さない。
  shared: boolean;
  // reserved は、有効な録画予約があること。枠を赤で囲む。
  reserved: boolean;
  // watchReserved は、ログイン中のユーザーが視聴予約したこと。時刻の前にベルの印を出す (設計書 4.8)。
  watchReserved: boolean;
};

// GuideGenre は、番組のジャンルの大分類と中分類。
export type GuideGenre = { readonly lv1: number; readonly lv2: number };

// GuideColumn は、番組表の 1 列 (1 つのサービス)。programs は開始時刻の順に並べる。
export type GuideColumn = {
  id: string;
  name: string;
  // logoUrl は、サービスのロゴの URL のパス。ロゴがなければ null。
  logoUrl: string | null;
  programs: GuideProgram[];
};

type Props = {
  window: GuideWindow;
  columns: GuideColumn[];
  // now は、いまの時刻。時間帯に入っていれば、線を引く。
  now: number;
  // receivedUntil は、番組表が届いている時刻。これより後は、まだ届いていないと案内する。省くと、案内しない。
  receivedUntil?: number;
  // scrollKey を変えると、日付が同じでも、scrollTime へスクロールし直す。
  scrollKey?: number;
  // scrollProgram は、scrollTime へスクロールするときに、横にも見えるようにする番組の ID。その番組の列を左端に寄せる。
  scrollProgram?: string | undefined;
  // scrollTime は、日付を変えたとき (と最初) に、上の方に出す時刻。null なら、それまで見ていた時間帯のままにする。
  scrollTime: number | null;
  // extendedHours は、深夜を 24 時以降 (28 時間表記) で表すかどうか (#486)。
  extendedHours?: boolean;
  // renderHeader は、列の見出し。省くと、サービス名 (押すとライブ視聴) と、そのチャンネルの数日分の番組表への入り口を出す。
  renderHeader?: (column: GuideColumn) => ReactNode;
  // headerAction は、既定の見出しの右に足す操作 (ピン留めなど。#485)。renderHeader を渡したときは使わない。
  headerAction?: (column: GuideColumn) => ReactNode;
  onSelect: (programId: string) => void;
};

const axisWidth = 44;
const headerHeight = 36;
// overscanPixels は、見えている範囲の上下に、余分に描く高さ。速くスクロールしても、空白が見えにくくする。
const overscanPixels = 600;

// GuideGrid は、横にサービス、縦に時刻を並べた番組表。
// サービスの数と番組の数が多くても滑らかに動くよう、横はサービスの列を、縦は列の中の番組を、見えている範囲だけ描く。
export function GuideGrid({
  window: day,
  columns,
  now,
  receivedUntil,
  scrollKey,
  scrollProgram,
  scrollTime,
  extendedHours = false,
  renderHeader,
  headerAction,
  onSelect,
}: Props) {
  const { t } = useTranslation();
  // チャンネルの幅と 1 分の高さは、端末ごとの密度の設定に従う (#483)。
  const [appearance] = useGuideAppearance();
  const { columnWidth, minuteHeight } = densities[appearance.density];
  const scroller = useRef<HTMLDivElement>(null);
  // マウスでドラッグして、番組表を動かせるようにする (#698)。
  useDragScroll(scroller);
  const [viewport, setViewport] = useState({ top: 0, height: 800 });
  // 列の番組の配置は、番組が変わったときだけ計算し直す。
  const layouts = useMemo(() => columns.map((column) => layoutColumn(column.programs)), [columns]);
  // 短い番組がある時間だけ高くした目盛り。番組表の位置は、すべてこれで計算する。
  const window = useMemo(
    () =>
      timeScale(
        { ...day, minuteHeight },
        columns.map((column, i) => ({ programs: column.programs, layout: layouts[i]! })),
      ),
    [day, columns, layouts, minuteHeight],
  );
  // windowRef は、最新の目盛り。番組を読み直しても、スクロールの位置は動かさないので、効果の依存には入れない。
  const windowRef = useRef(window);
  windowRef.current = window;
  // topOffset は、スクロールの上端の時刻の、その日の始まりからの経過時間。日付を変えたときに、同じ時間帯を出すのに使う。
  const topOffset = useRef<number | null>(null);

  const virtualizer = useVirtualizer({
    horizontal: true,
    count: columns.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => columnWidth,
    overscan: 2,
  });
  // 密度を変えたら、列の幅を測り直す。
  useEffect(() => {
    virtualizer.measure();
  }, [virtualizer, columnWidth]);

  // 縦のスクロールの位置を、描く番組を決めるために追う。描画は、フレームごとに 1 回にまとめる。
  useEffect(() => {
    const element = scroller.current;
    if (element === null) {
      return;
    }
    let frame = 0;
    const update = () => {
      frame = 0;
      topOffset.current = yToTime(windowRef.current, element.scrollTop) - windowRef.current.from;
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

  // 最初と、日付や scrollKey を変えたときに、scrollTime が上の方に来るようにスクロールする。
  // scrollTime がなければ、それまで見ていた時間帯 (その日の始まりからの経過時間) を、新しい日でも上端に出す。
  // 目盛りは日によって伸び方が違うので、位置ではなく時刻で合わせる。
  // scrollTimeRef は、最新の scrollTime。日付を変えたときだけ動かすので、効果の依存には入れない。
  const scrollTimeRef = useRef(scrollTime);
  scrollTimeRef.current = scrollTime;
  const scrollProgramRef = useRef(scrollProgram);
  scrollProgramRef.current = scrollProgram;
  const columnsRef = useRef(columns);
  columnsRef.current = columns;
  useEffect(() => {
    const element = scroller.current;
    if (element === null) {
      return;
    }
    const window = windowRef.current;
    const time = scrollTimeRef.current;
    if (time !== null) {
      element.scrollTop = Math.max(timeToY(window, time) - 60 * minuteHeightOf(window) * 0.5, 0);
      // 列は同じ幅で並ぶので、番組の列の番号から横の位置が決まる。
      const programId = scrollProgramRef.current;
      const index =
        programId === undefined
          ? -1
          : columnsRef.current.findIndex((column) =>
              column.programs.some((program) => program.id === programId),
            );
      if (index >= 0) {
        element.scrollLeft = index * columnWidth;
      }
    } else if (topOffset.current !== null) {
      element.scrollTop = timeToY(window, window.from + topOffset.current);
    }
  }, [day.from, scrollKey]);

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
  const columnBoxes = useMemo(
    () => columns.map((column, i) => stackedBoxes(window, column.programs, layouts[i]!)),
    [columns, layouts, window],
  );
  const totalWidth = axisWidth + virtualizer.getTotalSize();
  const nowY = now >= window.from && now < window.to ? timeToY(window, now) : undefined;
  const receivedY =
    receivedUntil !== undefined && receivedUntil < window.to
      ? timeToY(window, Math.max(receivedUntil, window.from))
      : undefined;

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
          <Box
            key={item.key}
            style={{
              position: 'absolute',
              top: 0,
              left: axisWidth + item.start,
              width: item.size,
              height: headerHeight,
              borderLeft: '1px solid var(--mantine-color-default-border)',
            }}
          >
            {renderHeader === undefined ? (
              <ServiceHeader column={columns[item.index]!} action={headerAction} />
            ) : (
              renderHeader(columns[item.index]!)
            )}
          </Box>
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
              // 正時の位置に線を引き、時刻の数字はその線の下に置く。
              style={{
                position: 'absolute',
                top: timeToY(window, hour),
                width: axisWidth,
                paddingTop: 2,
                borderTop: '1px solid var(--mantine-color-default-border)',
              }}
            >
              {String(guideHour(hour, day.from, extendedHours)).padStart(2, '0')}
            </Text>
          ))}
        </Box>
        {virtualizer.getVirtualItems().map((item) => (
          <GuideColumnView
            key={item.key}
            column={columns[item.index]!}
            layout={layouts[item.index]!}
            boxes={columnBoxes[item.index]!}
            left={axisWidth + item.start}
            width={item.size}
            visibleFrom={visibleFrom}
            visibleTo={visibleTo}
            extendedHours={extendedHours}
            onSelect={onSelect}
          />
        ))}
        {receivedY !== undefined && (
          // まだ届いていない時間帯は、斜線で塗り、横にスクロールしても見える位置に案内を出す。
          <Box
            style={{
              position: 'absolute',
              top: receivedY,
              left: axisWidth,
              width: totalWidth - axisWidth,
              height: height - receivedY,
              borderTop: '1px dashed var(--mantine-color-default-border)',
              background:
                'repeating-linear-gradient(135deg, transparent 0 8px, var(--mantine-color-default-hover) 8px 16px)',
            }}
          >
            <Text
              size="sm"
              c="dimmed"
              style={{
                position: 'sticky',
                top: headerHeight + 16,
                left: axisWidth + 16,
                display: 'inline-block',
                marginTop: 16,
                marginLeft: 16,
                paddingInline: 8,
                paddingBlock: 4,
                borderRadius: 'var(--mantine-radius-sm)',
                background: 'var(--mantine-color-body)',
              }}
            >
              {t('guide.notReceived')}
            </Text>
          </Box>
        )}
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
  column: GuideColumn;
  layout: ColumnLayout;
  boxes: ProgramBox[];
  left: number;
  width: number;
  visibleFrom: number;
  visibleTo: number;
  extendedHours: boolean;
  onSelect: (programId: string) => void;
};

function GuideColumnView({
  column,
  layout,
  boxes,
  left,
  width,
  visibleFrom,
  visibleTo,
  extendedHours,
  onSelect,
}: ColumnProps) {
  const { t } = useTranslation();
  const [appearance] = useGuideAppearance();
  const { start, end } = visibleRange(column.programs, layout, visibleFrom, visibleTo);

  return (
    <>
      {column.programs.slice(start, end).map((program, offset) => {
        const index = start + offset;
        const box = boxes[index]!;
        // 重なる番組は、列の幅を分けて横に並べる。
        const laneWidth = width / layout.laneCounts[index]!;

        return (
          <UnstyledButton
            key={program.id}
            data-program-id={program.id}
            onClick={() => onSelect(program.id)}
            title={
              program.shared
                ? t('guide.sharedProgram', {
                    name: toHalfWidth(splitProgramMarks(program.name).name),
                  })
                : toHalfWidth(splitProgramMarks(program.name).name)
            }
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
              background: program.shared
                ? 'transparent'
                : genreBackground(appearance, program.genre),
              borderLeft: '1px solid var(--mantine-color-default-border)',
              borderBottom: '1px solid var(--mantine-color-default-border)',
              // 録画予約した番組は、枠の内側を赤で囲む。
              boxShadow: program.reserved
                ? 'inset 0 0 0 2px var(--mantine-color-red-6)'
                : undefined,
              fontSize: 'var(--mantine-font-size-xs)',
              lineHeight: 1.35,
            }}
          >
            {!program.shared && (
              <span>
                {program.watchReserved && (
                  <BellRing
                    size={11}
                    role="img"
                    aria-label={t('watch.reserved')}
                    style={{
                      marginRight: 2,
                      verticalAlign: '-1px',
                      color: 'var(--mantine-color-mikan-7)',
                    }}
                  />
                )}
                <Text span size="xs" fw={700} mr={4}>
                  {guideTime(program.startAt, broadcastDay(program.startAt).from, extendedHours)}
                </Text>
                <ProgramTitle name={program.name} />
              </span>
            )}
          </UnstyledButton>
        );
      })}
    </>
  );
}

// ServiceHeader は、サービスの列の見出し。サービス名を押すとライブ視聴を開き、右のボタンでそのチャンネルの数日分の番組表を開く (#482)。
function ServiceHeader({
  column,
  action,
}: {
  column: GuideColumn;
  action?: (column: GuideColumn) => ReactNode;
}) {
  const { t } = useTranslation();

  return (
    <Group gap={0} wrap="nowrap" h={headerHeight}>
      <Anchor
        size="sm"
        fw={700}
        truncate
        c="inherit"
        underline="hover"
        title={t('guide.watchLive', { name: column.name })}
        renderRoot={(props) => (
          <Link {...props} to="/live/$serviceId" params={{ serviceId: column.id }} />
        )}
        style={{ flex: 1, minWidth: 0, lineHeight: `${headerHeight}px`, paddingInline: 6 }}
      >
        <ServiceLabel name={column.name} logoUrl={column.logoUrl} logoHeight="24px" />
      </Anchor>
      <Tooltip label={t('guide.channelWeek', { name: column.name })}>
        <ActionIcon
          variant="subtle"
          color="gray"
          size="sm"
          mr={4}
          aria-label={t('guide.channelWeek', { name: column.name })}
          renderRoot={(props) => (
            <Link {...props} to="/guide/channel/$serviceId" params={{ serviceId: column.id }} />
          )}
        >
          <CalendarRange size={14} aria-hidden />
        </ActionIcon>
      </Tooltip>
      {action?.(column)}
    </Group>
  );
}
