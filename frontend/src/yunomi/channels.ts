// neighborServices は、同じチャンネル種別のサービスのうち、current の前と次を返す (#493)。端では反対の端へ回る。
// services は、番組表と同じ並び順のサービス。同じ種別にほかのサービスがなければ、前も次も null にする。
export function neighborServices<T extends { id: string; channelType: { type: string } }>(
  services: readonly T[],
  current: string,
): { previous: T | null; next: T | null } {
  const self = services.find((service) => service.id === current);
  if (self === undefined) {
    return { previous: null, next: null };
  }
  const sameType = services.filter((service) => service.channelType.type === self.channelType.type);
  if (sameType.length < 2) {
    return { previous: null, next: null };
  }
  const index = sameType.indexOf(self);

  return {
    previous: sameType[(index - 1 + sameType.length) % sameType.length]!,
    next: sameType[(index + 1) % sameType.length]!,
  };
}
