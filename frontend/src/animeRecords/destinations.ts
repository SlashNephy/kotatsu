// 見たアニメの記録先 (#927)。

export type AnimeRecordDestinationName = 'annict' | 'anilist';

// destinationNames は、記録先の、画面に出す名前。サービスの名前なので、翻訳しない。
export const destinationNames: Record<AnimeRecordDestinationName, string> = {
  annict: 'Annict',
  anilist: 'AniList',
};

const callbackErrors = ['expired', 'mismatch', 'denied', 'unavailable', 'failed'] as const;

export type AnimeRecordCallbackError = (typeof callbackErrors)[number];

// animeRecordCallbackError は、記録先から戻ってきたときの error=<理由> を、画面に出す理由にする。
// 知らない理由は、失敗として扱う。理由には、トークンなどの中身は入らない。
export function animeRecordCallbackError(reason: string): AnimeRecordCallbackError {
  const name = reason.replace(/^anime_record_/, '');

  return (callbackErrors as readonly string[]).includes(name)
    ? (name as AnimeRecordCallbackError)
    : 'failed';
}
