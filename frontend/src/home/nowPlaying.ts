// ホームに出す、放送中の番組を選ぶ (設計書 10.3)。

import { sharesWithListed } from '../guide/sharedEvent';

export type GuideRowLike = {
  service: { id: string; name: string; logoUrl?: string | null };
  programs: readonly {
    id: string;
    name: string;
    startAt: string;
    endAt: string;
    sharedFrom?: { id: string } | null;
  }[];
};

export type NowPlaying = {
  serviceId: string;
  serviceName: string;
  serviceLogoUrl: string | null;
  programName: string;
  startAt: string;
  endAt: string;
};

// nowPlaying は、サービスごとに、now に放送している番組を返す。番組表の並びのままにする。
// 放送中の番組がないサービスは出さない。サブチャンネルがメインチャンネルと同じ番組を放送している (イベント共有) ときは、
// 元のサービスの番組だけを出す。元のサービスが一覧にない (番組表に出さない設定など) ときは、そのまま出す。
export function nowPlaying(rows: readonly GuideRowLike[], now: number): NowPlaying[] {
  const serviceIds = new Set(rows.map((row) => row.service.id));
  const playing: NowPlaying[] = [];
  for (const row of rows) {
    const program = currentProgram(row, now);
    if (program !== undefined && !sharesWithListed(program, serviceIds)) {
      playing.push(toNowPlaying(row, program));
    }
  }

  return playing;
}

// pinnedNowPlaying は、ピン留めしたサービスのうち、now に放送しているものを、ピン留めした順に返す (#485)。
// ピン留めしたサービスは、イベント共有のサブチャンネルでも省かない (ライブの一覧のピン留めのタブと同じ)。
export function pinnedNowPlaying(
  rows: readonly GuideRowLike[],
  pinned: readonly string[],
  now: number,
): NowPlaying[] {
  return pinned.flatMap((id) =>
    rows
      .filter((row) => row.service.id === id)
      .flatMap((row) => {
        const program = currentProgram(row, now);

        return program === undefined ? [] : [toNowPlaying(row, program)];
      }),
  );
}

function currentProgram(row: GuideRowLike, now: number) {
  return row.programs.find(
    (candidate) => Date.parse(candidate.startAt) <= now && now < Date.parse(candidate.endAt),
  );
}

function toNowPlaying(row: GuideRowLike, program: GuideRowLike['programs'][number]): NowPlaying {
  return {
    serviceId: row.service.id,
    serviceName: row.service.name,
    serviceLogoUrl: row.service.logoUrl ?? null,
    programName: program.name,
    startAt: program.startAt,
    endAt: program.endAt,
  };
}
