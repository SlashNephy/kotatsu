import { Badge } from '@mantine/core';

import { toHalfWidth } from './halfWidth';
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
// アプリの中で番組名を出すところは、これを使う。
export function ProgramTitle({ name }: { name: string }) {
  const program = splitProgramMarks(name);

  return (
    <>
      {toHalfWidth(program.name)}
      <ProgramMarkBadges marks={program.marks} />
    </>
  );
}
