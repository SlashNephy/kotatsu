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
