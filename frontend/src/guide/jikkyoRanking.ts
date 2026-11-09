type RankableRow = {
  service: { jikkyo?: { id: string; force?: number | null } | null };
};

// rankByJikkyoForce は、実況の勢いがある局を、勢いの強い順に返す (#490)。同じ勢いなら、渡した順を保つ。
// サブチャンネルは本放送と同じ実況のチャンネルを使い、勢いも同じになるので、実況のチャンネルごとに最初の局だけを残す。
export function rankByJikkyoForce<R extends RankableRow>(rows: readonly R[]): R[] {
  const ranked = rows
    .filter((row) => (row.service.jikkyo?.force ?? 0) > 0)
    .sort((a, b) => (b.service.jikkyo?.force ?? 0) - (a.service.jikkyo?.force ?? 0));
  const seen = new Set<string>();

  return ranked.filter((row) => {
    const id = row.service.jikkyo?.id;
    if (id === undefined || seen.has(id)) {
      return false;
    }
    seen.add(id);

    return true;
  });
}
