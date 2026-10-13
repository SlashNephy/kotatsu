import { Badge, Group } from '@mantine/core';

import { splitProgramMarks } from './programMarks';

// ProgramMarkBadges は、囲み文字の中身 (字、HV など) を、枠付きの印として並べる。
export function ProgramMarkBadges({ marks }: { marks: readonly string[] }) {
  return marks.map((mark) => (
    <Badge key={mark} size="md" fz="xs" variant="default" radius="sm" px={5} fw={500} tt="none">
      {mark}
    </Badge>
  ));
}

// ProgramTitle は、番組名から囲み文字を外し、番組名の後ろに印として並べる。囲み文字は、フォントによっては潰れて読めない。
export function ProgramTitle({ name }: { name: string }) {
  const program = splitProgramMarks(name);

  return (
    <Group component="span" gap={6} wrap="wrap" display="inline-flex">
      <span>{program.name}</span>
      <ProgramMarkBadges marks={program.marks} />
    </Group>
  );
}
