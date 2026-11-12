// 不具合の報告の組み立て (#1521、設計書 11.3)。GitHub の Issue のフォームを開く URL を、問い合わせのキーで中身を埋めて作る。
// キーは .github/ISSUE_TEMPLATE/bug_report.yml の欄の id。「起きていること」と「再現の手順」は、利用者が GitHub で書く。

// issueFormURL は、不具合の報告のフォームを開く URL の、問い合わせの前の部分。
export const issueFormURL = 'https://github.com/SlashNephy/kotatsu/issues/new';
// issueTemplate は、不具合の報告のテンプレートのファイル名。
export const issueTemplate = 'bug_report.yml';

// 決めるパラメーター (設計書 付録 A)。
// urlLimit は、フォームを開く URL の全体の字数の上限。GitHub は 8192 字以上の URL を 414 で断る。
// GitHub にログインしていないブラウザでは、ログインの画面へ送る応答が約 5740 字から 500 になるので、それより下に置く
// (2026-11-12 に Chromium で確かめた。curl では約 6960 字。ブラウザが送るヘッダーの分だけ短くなる)。
export const urlLimit = 5500;
// reportLineLimit は、報告に入れるときに 1 行を切る字数。
export const reportLineLimit = 400;
// titleLimit と environmentLimit は、それぞれ字数の上限と、URL エンコード後の長さの上限。
// 日本語は 1 字がエンコードで 9 字になるので、字数だけでは長さを抑えられない。
export const titleLimit = { chars: 120, encoded: 600 } as const;
export const environmentLimit = { chars: 1000, encoded: 1600 } as const;

export type Level = 'INFO' | 'WARN' | 'ERROR';

export type ReportLine = {
  // time は、epoch ミリ秒の時刻。
  time: number;
  level: Level;
  // inferred は、重さを本文の言葉から推し量ったかどうか。推し量った重さの印は、行に付けない。
  inferred: boolean;
  text: string;
};

export type ReportSource = {
  // name は、見出しに出す出どころの名前。
  name: string;
  // lines は、古い順の行。
  lines: readonly ReportLine[];
  // total は、出どころが持っていた行の数。
  total: number;
  // failure は、ログを取れなかった理由の一言。取れたら null。
  failure: string | null;
};

export type IssueFields = {
  title: string;
  version: string;
  environment: string;
  logs: string;
};

// toWellFormed は、壊れたサロゲート (片割れ) を置き換え文字 (U+FFFD) に直す。片割れが残ると、URL のエンコードが例外を投げる。
export function toWellFormed(text: string): string {
  const replacement = String.fromCharCode(0xfffd);
  let result = '';
  for (let i = 0; i < text.length; i++) {
    const code = text.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = text.charCodeAt(i + 1);
      if (next >= 0xdc00 && next <= 0xdfff) {
        result += String.fromCharCode(code, next);
        i++;
        continue;
      }
      result += replacement;
    } else if (code >= 0xdc00 && code <= 0xdfff) {
      result += replacement;
    } else {
      result += String.fromCharCode(code);
    }
  }

  return result;
}

// encodedLength は、URL の問い合わせの値にしたときの長さ。URLSearchParams と同じく、空白は + にする。
// 1 字ずつのエンコードなので、つないだ文字列の長さは、それぞれの長さの和になる。
export function encodedLength(text: string): number {
  return new URLSearchParams({ v: toWellFormed(text) }).toString().length - 2;
}

// truncate は、text を UTF-16 の単位で max 字までに切り、切ったら末尾に「…」を付ける。サロゲートペアの間では切らない。
export function truncate(text: string, max: number): string {
  if (text.length <= max) {
    return text;
  }
  let end = Math.max(max - 1, 0);
  const code = text.charCodeAt(end - 1);
  if (code >= 0xd800 && code <= 0xdbff) {
    end--;
  }

  return `${text.slice(0, end)}…`;
}

// truncateEncoded は、text を、字数の上限 chars と、エンコード後の長さの上限 encoded の両方に収まるよう切る。
export function truncateEncoded(
  text: string,
  { chars, encoded }: { chars: number; encoded: number },
): string {
  let result = truncate(toWellFormed(text), chars);
  if (encodedLength(result) <= encoded) {
    return result;
  }
  // 字の境目 (コードポイント) ごとに、収まる最も長いところを二分探索で探す。
  const points = Array.from(result);
  let low = 0;
  let high = points.length;
  while (low < high) {
    const mid = Math.ceil((low + high) / 2);
    if (encodedLength(`${points.slice(0, mid).join('')}…`) <= encoded) {
      low = mid;
    } else {
      high = mid - 1;
    }
  }
  result = `${points.slice(0, low).join('')}…`;

  return encodedLength(result) <= encoded ? result : '';
}

const pad = (value: number, length = 2) => String(value).padStart(length, '0');

// formatTime は、時刻を、端末の時刻の YYYY-MM-DD HH:MM:SS にする。
export function formatTime(time: number): string {
  const date = new Date(time);

  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

// formatLine は、報告の 1 行にする。重さが本物の warn と error のときだけ、印を付ける。maxLength で本文を切る (0 なら切らない)。
export function formatLine(line: ReportLine, maxLength = reportLineLimit): string {
  const mark = line.inferred || line.level === 'INFO' ? '' : ` [${line.level.toLowerCase()}]`;
  const text = toWellFormed(line.text.replace(/\r?\n/g, ' '));

  return `${formatTime(line.time)}${mark} ${maxLength > 0 ? truncate(text, maxLength) : text}`;
}

// heading は、出どころごとの見出しの行。入れた行数と、全体の行数を含む。
export const heading = (source: ReportSource, included: number) =>
  `== ${source.name} (${included} / ${source.total}) ==`;
// failureLine は、ログを取れなかった理由の行。
export const failureLine = (failure: string) => `(${failure})`;
// skippedLine は、URL の上限に入らずに省いた行の数を知らせる行。
export const skippedLine = (count: number) =>
  `(URL の上限に入らない ${count} 行を省きました。全部入りの報告は「コピー」か「ダウンロード」で取れます)`;
// trimmedLine は、書き足して上限を超えたときに、古い方から省いた行の数を知らせる行。
export const trimmedLine = (count: number) => `(古い方から ${count} 行を省きました)`;

const severity = (level: Level) => (level === 'ERROR' ? 2 : level === 'WARN' ? 1 : 0);

// selectLogs は、ログの欄に budget (エンコード後の長さ) まで入る行を選んで、ログの欄の文字列を返す。
//   1. 見出し、取れなかった理由、省いた行数を知らせる行の分を、先に確保する。省いた行数は、最大の桁数で見積もる。
//   2. 全部の行を、warn と error を先に、同じ重さなら新しい順に並べる。
//   3. 入る行は取り、入らない行は飛ばして次を試す。
//   4. 出すときは、出どころごとに古い順へ並べ直す。
export function selectLogs(sources: readonly ReportSource[], budget: number): string {
  const allLines = sources.reduce((sum, source) => sum + source.lines.length, 0);
  const maxCount = Math.max(allLines, ...sources.map((source) => source.total));
  const newline = encodedLength('\n');
  let reserved = encodedLength(skippedLine(10 ** String(allLines).length - 1)) + newline;
  for (const source of sources) {
    reserved += encodedLength(heading({ ...source, total: maxCount }, maxCount)) + newline;
    if (source.failure != null) {
      reserved += encodedLength(failureLine(source.failure)) + newline;
    }
  }

  const candidates = sources.flatMap((source, sourceIndex) =>
    source.lines.map((line, lineIndex) => ({
      sourceIndex,
      lineIndex,
      line,
      text: formatLine(line),
    })),
  );
  candidates.sort(
    (a, b) => severity(b.line.level) - severity(a.line.level) || b.line.time - a.line.time,
  );
  let remaining = budget - reserved;
  const chosen = sources.map(() => new Set<number>());
  for (const candidate of candidates) {
    const length = encodedLength(candidate.text) + newline;
    if (length <= remaining) {
      chosen[candidate.sourceIndex]!.add(candidate.lineIndex);
      remaining -= length;
    }
  }

  const out: string[] = [];
  let skipped = 0;
  sources.forEach((source, sourceIndex) => {
    const picked = chosen[sourceIndex]!;
    skipped += source.lines.length - picked.size;
    out.push(heading(source, picked.size));
    if (source.failure != null) {
      out.push(failureLine(source.failure));
    }
    source.lines.forEach((line, lineIndex) => {
      if (picked.has(lineIndex)) {
        out.push(formatLine(line));
      }
    });
  });
  if (skipped > 0) {
    out.unshift(skippedLine(skipped));
  }

  return out.join('\n');
}

// fullLogs は、1 行も削らない全部入りのログ。「コピー」と「ダウンロード」に使う。
export function fullLogs(sources: readonly ReportSource[]): string {
  return sources
    .flatMap((source) => [
      heading(source, source.lines.length),
      ...(source.failure != null ? [failureLine(source.failure)] : []),
      ...source.lines.map((line) => formatLine(line, 0)),
    ])
    .join('\n');
}

// buildURL は、各欄を問い合わせのキーで埋めた、フォームを開く URL を返す。
export function buildURL(fields: IssueFields): string {
  const query = new URLSearchParams({
    template: issueTemplate,
    title: toWellFormed(fields.title),
    version: toWellFormed(fields.version),
    environment: toWellFormed(fields.environment),
    logs: toWellFormed(fields.logs),
  });

  return `${issueFormURL}?${query.toString()}`;
}

// logsBudget は、ログの欄に入れられる長さ。URL の上限から、ログの欄を空にした URL の長さを引いた残り。
export function logsBudget(fields: Omit<IssueFields, 'logs'>, limit = urlLimit): number {
  return limit - buildURL({ ...fields, logs: '' }).length;
}

// limitFields は、タイトルと環境を、それぞれの上限で切る。
export function limitFields(fields: IssueFields): IssueFields {
  return {
    ...fields,
    title: truncateEncoded(fields.title, titleLimit),
    environment: truncateEncoded(fields.environment, environmentLimit),
  };
}

// fitURL は、利用者が書き足した欄で URL を作る。上限を超えたら、ログの先頭 (古い方) から行を削り、
// 先頭に、古い方から何行省いたかを 1 行で書く。先頭の 1 行も入らないときは、ログを空にする。
export function fitURL(fields: IssueFields, limit = urlLimit): { url: string; trimmed: number } {
  const limited = limitFields(fields);
  let url = buildURL(limited);
  if (url.length <= limit) {
    return { url, trimmed: 0 };
  }
  const lines = limited.logs.split('\n');
  for (let trimmed = 1; trimmed < lines.length; trimmed++) {
    url = buildURL({
      ...limited,
      logs: [trimmedLine(trimmed), ...lines.slice(trimmed)].join('\n'),
    });
    if (url.length <= limit) {
      return { url, trimmed };
    }
  }

  // ログが空なら、削った行は無い (タイトルや版が長くて上限を超えても、ログのせいではない)。
  return {
    url: buildURL({ ...limited, logs: '' }),
    trimmed: limited.logs === '' ? 0 : lines.length,
  };
}

// fullReport は、ダウンロードとコピーに使う、全部入りの報告の本文。
export function fullReport(fields: IssueFields): string {
  return [
    `# ${fields.title}`,
    '',
    '## 版',
    fields.version,
    '',
    '## 環境',
    fields.environment,
    '',
    '## ログ',
    '```text',
    fields.logs,
    '```',
    '',
  ].join('\n');
}

// reportFileName は、ダウンロードのファイル名。作った時刻を分まで入れる。
export function reportFileName(now: Date): string {
  return `kotatsu-report-${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}.md`;
}
