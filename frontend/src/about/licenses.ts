// /about の画面に並べる、Kotatsu が同梱するソフトウェアとデータの一覧 (設計書 10.3、付録 B、#1604)。

// LicenseText は、ライセンスの文書のファイル 1 つ。
export type LicenseText = { name: string; text: string };

// SoftwareEntry は、一覧の 1 行。licenses がないものは、文書の在りか (note) だけを示す。
export type SoftwareEntry = {
  name: string;
  version: string | null;
  license: string;
  url: string | null;
  licenses: LicenseText[];
  note: string | null;
};

// BundledPackage は、ビルドで書き出す licenses.json の 1 件 (vite.config.ts の bundledLicenses)。
type BundledPackage = {
  name: string;
  version: string;
  license: string;
  homepage: string | null;
  files: LicenseText[];
  generated: boolean;
};

// repositoryURL は、Kotatsu のリポジトリ。
export const repositoryURL = 'https://github.com/SlashNephy/kotatsu';

// sourceURL は、リポジトリの path を、revision (分からなければ main) で開く URL を返す。
export function sourceURL(path: string, revision: string | null): string {
  return `${repositoryURL}/blob/${revision || 'main'}/${path}`;
}

// fetchBundledPackages は、画面に同梱するパッケージの一覧を読む。開発サーバーのように書き出していなければ null を返す。
export async function fetchBundledPackages(
  generatedNote: string,
  fetcher: typeof fetch = fetch,
): Promise<SoftwareEntry[] | null> {
  const response = await fetcher(`${import.meta.env.BASE_URL}licenses.json`);
  if (!response.ok || !(response.headers.get('Content-Type') ?? '').includes('json')) {
    return null;
  }
  const packages = (await response.json()) as BundledPackage[];

  return packages.map((pkg) => ({
    name: pkg.name,
    version: pkg.version,
    license: pkg.license,
    url: pkg.homepage,
    licenses: pkg.files,
    note: pkg.generated ? generatedNote : null,
  }));
}

// filterEntries は、名前かライセンスに query を含む行だけを返す。大文字と小文字は区別しない。
export function filterEntries(entries: readonly SoftwareEntry[], query: string): SoftwareEntry[] {
  const needle = query.trim().toLowerCase();
  if (needle === '') {
    return [...entries];
  }

  return entries.filter(
    (entry) =>
      entry.name.toLowerCase().includes(needle) || entry.license.toLowerCase().includes(needle),
  );
}

// unlicenseText は、The Unlicense の本文。字幕のフォントは、配布物にライセンスの文書を含めないので、ここに置く。
const unlicenseText = `This is free and unencumbered software released into the public domain.

Anyone is free to copy, modify, publish, use, compile, sell, or
distribute this software, either in source code form or as a compiled
binary, for any purpose, commercial or non-commercial, and by any
means.

In jurisdictions that recognize copyright laws, the author or authors
of this software dedicate any and all copyright interest in the
software to the public domain. We make this dedication for the benefit
of the public at large and to the detriment of our heirs and
successors. We intend this dedication to be an overt act of
relinquishment in perpetuity of all present and future rights to this
software under copyright law.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS BE LIABLE FOR ANY CLAIM, DAMAGES OR
OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
OTHER DEALINGS IN THE SOFTWARE.

For more information, please refer to <https://unlicense.org>
`;

// DataEntryNotes は、手で並べる行の注記の文。翻訳した文を渡す。
export type DataEntryNotes = {
  inRepository: string;
  inImage: string;
};

// bundledData は、パッケージの情報を持たない同梱物 (フォントとデータの表) を返す。設計書の付録 B の表と合わせる。
// libaribcaption の表の著作権表示は、public/licenses/libaribcaption.txt に置いてあり、画面と同じオリジンから読む。
export function bundledData(
  libaribcaptionNotice: string | null,
  revision: string | null,
  notes: DataEntryNotes,
): SoftwareEntry[] {
  return [
    {
      name: 'Rounded M+ 1m WadaLab mix（字幕とデータ放送のフォント）',
      version: null,
      license: 'Unlicense',
      url: 'https://github.com/vivid-lapin/rounded-mplus-wadalab-mix',
      licenses: [{ name: 'UNLICENSE', text: unlicenseText }],
      note: null,
    },
    {
      name: 'libaribcaption（字幕の外字の既定の表）',
      version: null,
      license: 'MIT',
      url: 'https://github.com/xqq/libaribcaption',
      licenses:
        libaribcaptionNotice === null
          ? []
          : [{ name: 'libaribcaption.txt', text: libaribcaptionNotice }],
      note: null,
    },
    {
      name: 'SecLists（パスワードのブロックリスト）',
      version: null,
      license: 'MIT',
      url: sourceURL('backend/internal/lib/password/blocklist.LICENSE', revision),
      licenses: [],
      note: notes.inRepository,
    },
    {
      name: 'KonomiTV（実況チャンネルの対応表）',
      version: null,
      license: 'MIT',
      url: sourceURL('backend/internal/usecase/jikkyo/table.go', revision),
      licenses: [],
      note: notes.inRepository,
    },
    {
      name: 'aribb24.js（ARIBの追加漢字と追加記号の表）',
      version: null,
      license: 'MIT',
      url: sourceURL('backend/internal/lib/ts/aribsymbols.go', revision),
      licenses: [],
      note: notes.inRepository,
    },
  ];
}

// imagePrograms は、-ffmpeg イメージにだけ同梱するプログラムを返す。ライセンスの文書とソースの入手先は、イメージの中に置く。
export function imagePrograms(note: string): SoftwareEntry[] {
  return [
    {
      name: 'FFmpeg',
      version: null,
      license: 'GPL-2.0-or-later',
      url: 'https://ffmpeg.org/',
      licenses: [],
      note,
    },
    {
      name: 'tsreadex',
      version: null,
      license: 'MIT',
      url: 'https://github.com/xtne6f/tsreadex',
      licenses: [],
      note,
    },
    {
      name: 'Intel Media Driver（amd64のみ）',
      version: null,
      license: 'MIT',
      url: 'https://github.com/intel/media-driver',
      licenses: [],
      note,
    },
  ];
}
