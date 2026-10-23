// formatVersion は、版とコミットの先頭 7 文字を、1.2.3 (abcdef0) のように返す。kotatsu --version と同じ形にする。
export function formatVersion(v: {
  version: string;
  revision: string | null | undefined;
  modified: boolean;
}): string {
  if (v.revision == null || v.revision === '') {
    return v.version;
  }

  return `${v.version} (${v.revision.slice(0, 7)}${v.modified ? '-dirty' : ''})`;
}
