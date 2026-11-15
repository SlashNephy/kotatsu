// 見逃し配信のサービスで作品名を探すリンク (#1354)。配信しているかは、配信のサービスに問い合わせないと分からないので、
// Kotatsu からは問い合わせず、リンクを押したときに、ブラウザーが配信のサービスの検索へ移るだけにする。

// StreamingService は、見逃し配信のサービス。tver は民放の地上波、nhk は NHK ONE。
export type StreamingService = 'tver' | 'nhk';

// StreamingSearch は、配信のサービスと、そのサービスで作品名を探す URL。
export type StreamingSearch = { readonly service: StreamingService; readonly url: string };

type Service = { readonly name: string; readonly channelType: { readonly type: string } };

// isNHK は、サービス名から NHK の局かどうかを返す。全角で名乗る局もあるので、NFKC で半角に寄せてから見る。
function isNHK(name: string): boolean {
  return name.normalize('NFKC').toUpperCase().startsWith('NHK');
}

// streamingSearch は、番組の局 service と作品名 title から、見逃し配信のサービスで作品名を探す URL を返す。
// NHK の局 (地上波と BS) は NHK ONE、NHK でない地上波の局は TVer で探す。それ以外の局と、作品名が空のときは null を返す。
export function streamingSearch(service: Service, title: string): StreamingSearch | null {
  const q = title.trim();
  if (q === '') {
    return null;
  }
  const type = service.channelType.type;
  if (isNHK(service.name) && (type === 'GR' || type === 'BS')) {
    return { service: 'nhk', url: `https://www.web.nhk/search?query=${encodeURIComponent(q)}` };
  }
  if (type === 'GR' && !isNHK(service.name)) {
    return { service: 'tver', url: `https://tver.jp/search/${encodeURIComponent(q)}` };
  }

  return null;
}
