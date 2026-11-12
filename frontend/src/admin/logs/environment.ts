// 不具合の報告の「環境」の欄 (#1521、設計書 11.3)。

// allRoles は、Kotatsu のロール。all の構成では、1 つのプロセスがすべてを動かす。
export const allRoles = ['tenban', 'neko', 'mikan', 'nekonote', 'kyusu'] as const;

// otherRoles は、このプロセスが動かしていないロールを返す。空でなければ、ロールを分けた構成で、ほかのロールのログは画面に出せない。
export function otherRoles(roles: readonly string[]): string[] {
  return allRoles.filter((role) => !roles.includes(role));
}

export type EnvironmentSource = {
  // kind は、出どころの種類 (KOTATSU か TUNER_SERVER)。
  kind: string;
  version: string;
  os: string;
  arch: string;
  runtime: string;
};

export type EnvironmentInput = {
  sources: readonly EnvironmentSource[];
  roles: readonly string[];
  tuners: readonly { name: string; types: readonly string[] }[];
  userAgent: string;
};

const joinParts = (parts: string[]) => parts.filter((part) => part !== '').join(', ');

// platform は、OS とアーキテクチャを「linux/amd64」の形にする。
const platform = (source: EnvironmentSource) =>
  [source.os, source.arch].filter((part) => part !== '').join('/');

// environmentText は、環境の欄の文を作る。
export function environmentText({ sources, roles, tuners, userAgent }: EnvironmentInput): string {
  const lines: string[] = [];
  for (const source of sources) {
    if (source.kind === 'KOTATSU') {
      const others = otherRoles(roles);
      lines.push(
        `- Kotatsu: ${joinParts([source.version || 'unknown', platform(source), source.runtime])}`,
      );
      lines.push(
        `- 構成: ${others.length === 0 ? 'all' : `split (この報告は ${roles.join(', ')} のログだけ)`}`,
      );
    } else {
      lines.push(
        `- チューナーサーバー: ${joinParts([source.runtime, source.version || 'unknown', platform(source)])}`,
      );
    }
  }
  if (tuners.length > 0) {
    // 同じ名前と種類のチューナーは、まとめて数える。
    const counts = new Map<string, number>();
    for (const tuner of tuners) {
      const key = `${tuner.name} (${tuner.types.join(', ')})`;
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    lines.push(
      `- チューナー: ${[...counts].map(([key, count]) => (count > 1 ? `${key} × ${count}` : key)).join('; ')}`,
    );
  }
  lines.push(`- ブラウザ: ${userAgent}`);

  return lines.join('\n');
}
