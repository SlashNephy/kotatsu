import { Badge } from '@mantine/core';

import { splitProgramMarks } from './programMarks';

// ProgramMarkBadges は、囲み文字の中身 (字、HV など) を、枠付きの印として並べる。
// 文字の流れに乗せるので、省略記号で切る行 (truncate や lineClamp) の中にも置ける。
export function ProgramMarkBadges({ marks }: { marks: readonly string[] }) {
  return marks.map((mark) => (
    <Badge
      key={mark}
      size="md"
      fz="xs"
      variant="default"
      radius="sm"
      px={5}
      fw={500}
      tt="none"
      ml={4}
      // 番組名の文の中 (<p> など) に置くので、インラインの要素にする。
      component="span"
      style={{ verticalAlign: 'middle' }}
    >
      {mark}
    </Badge>
  ));
}

// ProgramTitle は、番組名から囲み文字を外し、番組名の後ろに印として並べる。囲み文字は、フォントによっては潰れて読めない。
// アプリの中で番組名を出すところは、これを使う。
export function ProgramTitle({ name }: { name: string }) {
  const program = splitProgramMarks(name);

  return (
    <>
      {program.name}
      <ProgramMarkBadges marks={program.marks} />
    </>
  );
}
