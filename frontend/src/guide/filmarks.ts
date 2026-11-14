// Filmarks で作品名を探すリンク (#1650)。Filmarks には記録を送る公開の API がないので、記録しやすくする導線だけを置く。
// Kotatsu からは問い合わせず、リンクを押したときに、ブラウザーが Filmarks へ移るだけにする。

// Genre は、ARIB STD-B10 の大分類 (lv1) と中分類 (lv2)。
type Genre = { readonly lv1: number; readonly lv2: number };

// dramaGenre と movieGenre は、ドラマと映画の大分類。animeMovieSubGenre は、映画のうちアニメの中分類。
const dramaGenre = 0x3;
const movieGenre = 0x6;
const animeMovieSubGenre = 0x2;

// filmarksSearchURL は、ドラマか映画の番組の作品名 title を、Filmarks で探す URL を返す。
// ジャンルは並びの順に見て、先に当たったほうで探す。アニメの映画と、しょぼいカレンダーの話に結びついたアニメ (anime) では、
// アニメの記録先へのリンクがあるので、null を返す。ドラマでも映画でもないときと、作品名が空のときも null を返す。
export function filmarksSearchURL(
  genres: readonly Genre[],
  title: string,
  anime: boolean,
): string | null {
  const q = title.trim();
  if (anime || q === '') {
    return null;
  }
  for (const genre of genres) {
    if (genre.lv1 === dramaGenre) {
      return `https://filmarks.com/search/dramas?q=${encodeURIComponent(q)}`;
    }
    if (genre.lv1 === movieGenre && genre.lv2 !== animeMovieSubGenre) {
      return `https://filmarks.com/search/movies?q=${encodeURIComponent(q)}`;
    }
  }

  return null;
}
