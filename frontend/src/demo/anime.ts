import type { DemoProgram } from './epg';

// animeEpisodePattern は、デモの番組名の話数 (「#3」や「第3話」)。
const animeEpisodePattern = /#(\d+)|第(\d+)話/;

// demoAnimeEpisodes は、デモの番組に結びついた話を返す (設計書 3.8)。tenban の番組名の解析と同じく、アニメの番組名の話数を読む。
// デモの番組は架空なので、しょぼいカレンダーや Annict の作品には結びつけない。
export function demoAnimeEpisodes(program: Pick<DemoProgram, 'name' | 'genres'>) {
  if (!program.genres.some((genre) => genre.lv1 === 7)) {
    return [];
  }
  const match = animeEpisodePattern.exec(program.name);
  const number = Number(match?.[1] ?? match?.[2]);
  if (match === null || !Number.isFinite(number)) {
    return [];
  }

  return [
    {
      source: 'TITLE',
      syobocalTid: null,
      syobocalUrl: null,
      countStart: number,
      countEnd: number,
      subtitle: null,
      first: number === 1,
      last: false,
      works: [],
    },
  ];
}

// demoAnimeRecordSettings は、デモの、見たアニメの記録の設定 (#927)。デモは外部へ問い合わせないので、連携は使わない。
export function demoAnimeRecordSettings() {
  return {
    enabled: false,
    thresholdPercent: 20,
    watchingOnFirst: false,
    watchingOnEpisode: false,
    watchedOnLast: false,
    destinations: (['ANNICT', 'ANILIST'] as const).map((destination) => ({
      destination,
      available: false,
      connection: null,
    })),
  };
}

// demoAnimeRecordApps は、デモの、記録先に登録したアプリ (#927)。デモでは登録しない。
export function demoAnimeRecordApps(origin: string) {
  return (['ANNICT', 'ANILIST'] as const).map((destination) => ({
    destination,
    clientId: null,
    configured: false,
    redirectUri: `${origin}/auth/anime-records/${destination.toLowerCase()}/callback`,
    updatedAt: null,
  }));
}
