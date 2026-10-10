import { checkStrength } from '../src/password/zxcvbn';

// goja から呼ぶ入口。Go は、入力を JSON の文字列で渡し、結果を JSON の文字列で受け取る。
// goja のオブジェクトの受け渡しより、JSON の方が型の食い違いを起こしにくい。
declare global {
  var kotatsuCheckStrength: (input: string) => string;
}

globalThis.kotatsuCheckStrength = (input: string): string => {
  const { password, userInputs } = JSON.parse(input) as { password: string; userInputs: string[] };

  return JSON.stringify(checkStrength(password, userInputs));
};
