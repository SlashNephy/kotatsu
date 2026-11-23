import {
  ARIBB24ActiveCoordinatePositionSetToken,
  ARIBB24CharacterCompositionDotDesignationToken,
  ARIBB24CharacterToken,
  ARIBB24JapaneseInitialParserState,
  ARIBB24MiddleSizeToken,
  ARIBB24NormalSizeToken,
  ARIBB24Parser,
  ARIBB24SetDisplayFormatToken,
  ARIBB24SetDisplayPositionToken,
  ARIBB24SetHorizontalSpacingToken,
  ARIBB24SetVerticalSpacingToken,
  type ARIBB24Token,
} from 'aribb24.js';
import { describe, expect, test } from 'vitest';

// aribb24.js のパーサーに当てたパッチ (patches/aribb24.js@2.0.34.patch) の振る舞いを確かめる。
// 表示位置 (SDP) を変えても、書き込む位置は画面の上で動かない。

const chars = (text: string): ARIBB24Token[] =>
  Array.from(text, (character) => ARIBB24CharacterToken.from(character));

// placed は、解析した字幕の文字と、画面の上の左端の x、基準線の y を返す。
function placed(tokens: ARIBB24Token[]) {
  const parser = new ARIBB24Parser(ARIBB24JapaneseInitialParserState, { magnification: 1 });

  return parser
    .parse(tokens)
    .filter((token) => token.tag === 'Character')
    .map((token) => ({
      character: token.character,
      x: token.state.margin[0] + token.state.position[0],
      y: token.state.margin[1] + token.state.position[1],
    }));
}

const header: ARIBB24Token[] = [
  ARIBB24SetDisplayFormatToken.from(840, 480),
  ARIBB24SetDisplayPositionToken.from(60, 30),
  ARIBB24CharacterCompositionDotDesignationToken.from(36, 36),
  ARIBB24SetHorizontalSpacingToken.from(4),
  ARIBB24SetVerticalSpacingToken.from(24),
];

describe('ARIBB24Parser の表示位置 (SDP)', () => {
  test.each([
    {
      // 放送の字幕から写した並び。カッコだけを広い領域に置き、本文の領域を狭めてから、カッコの続きに書く。
      name: '書き込む位置を決めた後に変えても、文字は続きに並ぶ',
      tokens: [
        ...header,
        ARIBB24ActiveCoordinatePositionSetToken.from(180, 449),
        ARIBB24MiddleSizeToken.from(),
        ...chars('《'),
        ARIBB24SetDisplayFormatToken.from(620, 480),
        ARIBB24SetDisplayPositionToken.from(170, 30),
        ARIBB24NormalSizeToken.from(),
        ...chars('俺は'),
        ARIBB24ActiveCoordinatePositionSetToken.from(290, 509),
        ...chars('一緒に生きていきたい'),
        ARIBB24SetDisplayFormatToken.from(840, 480),
        ARIBB24SetDisplayPositionToken.from(60, 30),
        ARIBB24MiddleSizeToken.from(),
        ...chars('》'),
      ],
      expected: [
        { character: '《', x: 180, y: 449 },
        // 半角の《 (幅 20) の右に続く。
        { character: '俺', x: 200, y: 449 },
        { character: '一', x: 290, y: 509 },
        // 全角 10 文字 (幅 400) の右に続く。
        { character: '》', x: 690, y: 509 },
      ],
    },
    {
      name: '書き込む位置を決める前に変えたら、新しい領域の左上から書く',
      tokens: [...header, ARIBB24SetDisplayPositionToken.from(170, 90), ...chars('俺')],
      expected: [{ character: '俺', x: 170, y: 90 + 59 }],
    },
  ])('$name', ({ tokens, expected }) => {
    const characters = new Set(expected.map(({ character }) => character));

    expect(placed(tokens).filter(({ character }) => characters.has(character))).toEqual(expected);
  });
});
