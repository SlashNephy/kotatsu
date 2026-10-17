import type { GuideColumn, GuideProgram } from './GuideGrid';
import { broadcastDay } from './layout';

// channelWeekDays は、1 つのチャンネルの番組表に並べる日数。EPG が届く 8 日分にする (#482)。
export const channelWeekDays = 8;

// ChannelDay は、1 つのチャンネルの番組表の 1 列 (1 つの放送日)。
export type ChannelDay = GuideColumn & {
  // from は、その放送日の始まり (4 時) の時刻。
  from: number;
};

// channelWeekColumns は、1 つのチャンネルの番組を、今日から days 日分の放送日ごとの列に分ける。
// 番組表 (GuideGrid) は、すべての列が同じ時間帯 (今日の放送日) を共有するので、各列の番組の時刻を、今日の放送日へずらす。
// 時刻の表示は時と分だけなので、ずらしても同じ時刻に見える。放送日をまたぐ番組は、始まった日の列に入れる。
export function channelWeekColumns(
  today: number,
  serviceId: string,
  label: (from: number) => string,
  programs: readonly GuideProgram[],
  days = channelWeekDays,
): ChannelDay[] {
  const start = broadcastDay(today).from;
  const columns: ChannelDay[] = [];
  let dayFrom = start;
  for (let i = 0; i < days; i++) {
    // 夏時間のある地域では、放送日の長さが 23 時間か 25 時間になるので、次の日の始まりは、その日の終わりから求める。
    const window = broadcastDay(dayFrom);
    const shift = window.from - start;
    columns.push({
      id: `${serviceId}:${i}`,
      name: label(window.from),
      logoUrl: null,
      from: window.from,
      programs: programs
        .filter((program) => program.startAt >= window.from && program.startAt < window.to)
        .map((program) => ({
          ...program,
          startAt: program.startAt - shift,
          endAt: program.endAt - shift,
        })),
    });
    dayFrom = window.to;
  }

  return columns;
}
