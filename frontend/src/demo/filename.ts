// デモのファイル名のテンプレート。tenban は Go の text/template で組み立てる (internal/reserve/filename.go) が、
// デモでは、画面の説明に載せている書き方だけを置き換える。

export type FileNameData = {
  name: string;
  serviceName: string;
  channelType: string;
  channel: string;
  eventId: number;
  startAt: number;
  endAt: number;
};

export const defaultFileNameTemplate =
  '{{.StartAt.Format "2006-01-02 1504"}} {{.ServiceName}} {{.Name}}';

export const fileNameTemplateItems: { snippet: string; description: string }[] = [
  { snippet: '{{.Name}}', description: '番組名' },
  { snippet: '{{half .Name}}', description: '番組名 (英数字と空白を半角にする)' },
  { snippet: '{{.ServiceName}}', description: 'サービス名' },
  { snippet: '{{half .ServiceName}}', description: 'サービス名 (英数字と空白を半角にする)' },
  { snippet: '{{.ChannelType}}', description: '放送波 (GR、BS、CS など)' },
  { snippet: '{{.Channel}}', description: 'チャンネル' },
  { snippet: '{{.EventID}}', description: '番組の eventId (時刻予約では 0)' },
  { snippet: '{{.StartAt.Format "2006"}}', description: '開始の年 (4 桁)' },
  { snippet: '{{.StartAt.Format "06"}}', description: '開始の年 (2 桁)' },
  { snippet: '{{.StartAt.Format "01"}}', description: '開始の月 (2 桁)' },
  { snippet: '{{.StartAt.Format "02"}}', description: '開始の日 (2 桁)' },
  { snippet: '{{weekdayJa .StartAt}}', description: '開始の曜日' },
  { snippet: '{{.StartAt.Format "15"}}', description: '開始の時 (24 時間制、2 桁)' },
  { snippet: '{{.StartAt.Format "04"}}', description: '開始の分 (2 桁)' },
  {
    snippet: '{{.EndAt.Format "1504"}}',
    description: '終了の時分 (EndAt も StartAt と同じ書き方)',
  },
];

export const fileNameTemplatePresets: { name: string; template: string }[] = [
  { name: '既定値', template: defaultFileNameTemplate },
  {
    name: 'EPGStation と同じ',
    template:
      '[{{.StartAt.Format "06-01-02"}}({{weekdayJa .StartAt}})_{{.StartAt.Format "15-04"}}][{{.ChannelType}}_{{half .ServiceName}}]{{half .Name}}',
  },
];

// sampleFileNameData は、番組を指定しないときの例 (tenban の SampleFileNameData と同じ)。
export const sampleFileNameData: FileNameData = {
  name: 'こたつでみかんＳＰ　＃２',
  serviceName: 'ＫＯＴＡＴＳＵ　ＴＶ１',
  channelType: 'GR',
  channel: '16',
  eventId: 1234,
  startAt: Date.parse('2026-10-11T15:30:00Z'),
  endAt: Date.parse('2026-10-11T16:00:00Z'),
};

const jstOffset = 9 * 60 * 60 * 1000;

function half(text: string): string {
  return text.replace(/[　Ａ-Ｚａ-ｚ０-９＃＄％＆＊＋－．／：；＝＠＿]/gu, (char) =>
    char === '　' ? ' ' : String.fromCodePoint(char.codePointAt(0)! - 0xfee0),
  );
}

// format は、Go の時刻のレイアウトのうち、年、月、日、時、分、秒を置き換える。
function format(time: number, layout: string): string {
  const local = new Date(time + jstOffset);
  const pad = (value: number) => String(value).padStart(2, '0');
  const parts: Record<string, string> = {
    '2006': String(local.getUTCFullYear()),
    '06': pad(local.getUTCFullYear() % 100),
    '01': pad(local.getUTCMonth() + 1),
    '02': pad(local.getUTCDate()),
    '15': pad(local.getUTCHours()),
    '04': pad(local.getUTCMinutes()),
    '05': pad(local.getUTCSeconds()),
  };

  return layout.replace(/2006|06|01|02|15|04|05/gu, (token) => parts[token]!);
}

// renderFileName は、テンプレートを data で埋めたファイル名を返す。デモで扱えない書き方なら、エラーの文を返す。
export function renderFileName(
  template: string,
  data: FileNameData,
): { fileName: string | null; error: string | null } {
  let error: string | null = null;
  const weekdays = ['日', '月', '火', '水', '木', '金', '土'];
  const fileName = template.replace(/\{\{\s*(.*?)\s*\}\}/gu, (_whole, action: string) => {
    const time = action.match(/^\.(StartAt|EndAt)\.Format\s+"([^"]*)"$/u);
    if (time !== null) {
      return format(time[1] === 'StartAt' ? data.startAt : data.endAt, time[2]!);
    }
    const fields: Record<string, string> = {
      '.Name': data.name,
      'half .Name': half(data.name),
      '.ServiceName': data.serviceName,
      'half .ServiceName': half(data.serviceName),
      '.ChannelType': data.channelType,
      '.Channel': data.channel,
      '.EventID': String(data.eventId),
      'weekdayJa .StartAt': weekdays[new Date(data.startAt + jstOffset).getUTCDay()]!,
    };
    const value = fields[action];
    if (value === undefined) {
      error = `デモでは使えない書き方です: {{${action}}}`;

      return '';
    }

    return value;
  });

  return error === null ? { fileName, error: null } : { fileName: null, error };
}
