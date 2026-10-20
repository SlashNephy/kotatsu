import type { ComboboxItem, ComboboxItemGroup, ComboboxLikeRenderOptionInput } from '@mantine/core';
import type { ReactNode } from 'react';

import { ServiceLabel } from './ServiceLabel';

type ServiceLike = {
  id: string;
  name: string;
  logoUrl?: string | null;
  channelType: { displayName: string };
};

// serviceGroups は、サービスを選ぶドロップダウン (Select、MultiSelect) の選択肢を、放送波 (チャンネル種別) ごとにまとめて返す。
// 放送波は、最初に現れた順に並べ、その中のサービスは、渡した順のままにする。label は、選択肢に出す名前を決める。
export function serviceGroups<S extends ServiceLike>(
  services: readonly S[],
  label: (service: S) => string = (service) => service.name,
): ComboboxItemGroup<ComboboxItem>[] {
  const groups = new Map<string, ComboboxItem[]>();
  for (const service of services) {
    const items = groups.get(service.channelType.displayName) ?? [];
    items.push({ value: service.id, label: label(service) });
    groups.set(service.channelType.displayName, items);
  }

  return [...groups].map(([group, items]) => ({ group, items }));
}

// renderServiceOption は、サービスを選ぶドロップダウンの選択肢を、ロゴを添えたサービス名で描く関数を返す。
// 選択肢の value はサービスの ID、label は出す名前にする。services は、ID からロゴを引くのに使う。
export function renderServiceOption(
  services: readonly { id: string; logoUrl?: string | null }[],
): (input: ComboboxLikeRenderOptionInput<ComboboxItem>) => ReactNode {
  const logos = new Map(services.map((service) => [service.id, service.logoUrl]));

  return ({ option }) => <ServiceLabel name={option.label} logoUrl={logos.get(option.value)} />;
}
