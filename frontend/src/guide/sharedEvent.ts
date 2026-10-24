// イベント共有 (サブチャンネルがメインチャンネルと同じ番組を流していること) で、一覧から省く番組を見分ける。

// sharesWithListed は、番組が、一覧にあるほかのサービスの番組と同じイベントを流しているかどうかを返す。
// 元のサービスが一覧にない (番組表に出さない設定など) ときは、省くと見られなくなるので、false を返す。
export function sharesWithListed(
  program: { sharedFrom?: { id: string } | null } | undefined,
  listedServiceIds: ReadonlySet<string>,
): boolean {
  const from = program?.sharedFrom;

  return from != null && listedServiceIds.has(from.id);
}

type SharedLike = { sharedFrom?: { id: string } | null };

// MarkedColumn は、番組に shared の印を付けた列。
type MarkedColumn<C extends { programs: readonly SharedLike[] }> = Omit<C, 'programs'> & {
  programs: (C['programs'][number] & { shared: boolean })[];
};

// collapseSharedColumns は、番組表の列から、その日ずっと元のサービスと同じ番組を流している列 (サブチャンネル) を外し、
// 残った列では、元のサービスと同じ番組に shared の印を付ける。番組のない列は、そのまま残す。
export function collapseSharedColumns<C extends { id: string; programs: readonly SharedLike[] }>(
  columns: readonly C[],
): MarkedColumn<C>[] {
  const listed = new Set(columns.map((column) => column.id));
  const marked: MarkedColumn<C>[] = columns.map((column) => ({
    ...column,
    programs: column.programs.map((program) => ({
      ...program,
      shared: sharesWithListed(program, listed),
    })),
  }));

  return marked.filter(
    (column) => column.programs.length === 0 || column.programs.some((program) => !program.shared),
  );
}
