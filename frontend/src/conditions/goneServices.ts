import type { ServiceOption } from './ConditionsFields';

// withSelectedGoneServices は、選択肢のサービスに、ルールが選んでいる、上流から消えたサービスを足す (#1973)。
// 消えたサービスは選択肢の一覧に無いので、足さないと、選んでいるのに名前の代わりに ID が出る。
export function withSelectedGoneServices(
  services: readonly ServiceOption[],
  conditions:
    | {
        readonly services: readonly ServiceOption[];
        readonly excludeServices: readonly ServiceOption[];
      }
    | null
    | undefined,
): readonly ServiceOption[] {
  if (conditions == null) {
    return services;
  }
  const known = new Set(services.map((service) => service.id));
  const gone: ServiceOption[] = [];
  for (const service of [...conditions.services, ...conditions.excludeServices]) {
    if (service.gone === true && !known.has(service.id)) {
      known.add(service.id);
      gone.push(service);
    }
  }

  return gone.length === 0 ? services : [...services, ...gone];
}
