import { Badge } from '@mantine/core';

import { toHalfWidth } from './halfWidth';
import { highlightRanges, type HighlightKeyword } from './highlight';
import classes from './ProgramMarks.module.css';
import { splitProgramMarks } from './programMarks';

// ProgramMarkBadges は、囲み文字の中身 (字、HV など) を、枠付きの印として並べる。
// 文字の流れに乗せるので、省略記号で切る行 (truncate や lineClamp) の中にも置ける。
export function ProgramMarkBadges({ marks }: { marks: readonly string[] }) {
  if (marks.length === 0) {
    return null;
  }

  return (
    <span className={classes.marks}>
      {marks.map((mark) => (
        <Badge
          key={mark}
          size="md"
          fz="xs"
          variant="default"
          radius="sm"
          px={5}
          fw={500}
          tt="none"
          // 番組名の文の中 (<p> など) に置くので、インラインの要素にする。
          component="span"
        >
          {mark}
        </Badge>
      ))}
    </span>
  );
}

// ProgramTitle は、番組名から囲み文字を外し、番組名の後ろに印として並べる。全角の英数字は半角にする。囲み文字は、フォントによっては潰れて読めない。
// アプリの中で番組名を出すところは、これを使う。highlights を渡すと、番組名のうち、そのキーワードに当たった所に印を付ける (#966)。
export function ProgramTitle({
  name,
  highlights = [],
}: {
  name: string;
  highlights?: readonly HighlightKeyword[];
}) {
  const program = splitProgramMarks(name);
  const title = toHalfWidth(program.name);

  return (
    <>
      <HighlightedText text={title} keywords={highlights} />
      <ProgramMarkBadges marks={program.marks} />
    </>
  );
}

// HighlightedText は、text のうち、keywords に当たった所を <mark> で囲んで出す。
function HighlightedText({
  text,
  keywords,
}: {
  text: string;
  keywords: readonly HighlightKeyword[];
}) {
  const ranges = keywords.length === 0 ? [] : highlightRanges(text, keywords);
  if (ranges.length === 0) {
    return text;
  }
  const parts = [];
  let at = 0;
  for (const range of ranges) {
    parts.push(text.slice(at, range.start));
    parts.push(
      <mark key={range.start} className={classes.highlight}>
        {text.slice(range.start, range.end)}
      </mark>,
    );
    at = range.end;
  }
  parts.push(text.slice(at));

  return parts;
}
