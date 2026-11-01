/**
 * @generated SignedSource<<3b4d4e9251d63a0da59b78f5470d63d7>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type KeywordMatch = "ALL" | "ANY" | "%future added value";
export type WatchRuleInput = {
  conditions: ProgramConditionsInput;
  enabled?: boolean;
  name: string;
};
export type ProgramConditionsInput = {
  channelTypes?: ReadonlyArray<string> | null | undefined;
  excludeGenres?: ReadonlyArray<GenreInput> | null | undefined;
  excludeKeywords?: ReadonlyArray<KeywordInput> | null | undefined;
  excludeMarks?: ReadonlyArray<string> | null | undefined;
  excludeServices?: ReadonlyArray<string> | null | undefined;
  freeOnly?: boolean | null | undefined;
  genres?: ReadonlyArray<GenreInput> | null | undefined;
  keywordMatch?: KeywordMatch;
  keywords?: ReadonlyArray<KeywordInput> | null | undefined;
  marks?: ReadonlyArray<string> | null | undefined;
  maxDurationSeconds?: number | null | undefined;
  minDurationSeconds?: number | null | undefined;
  services?: ReadonlyArray<string> | null | undefined;
  timeRanges?: ReadonlyArray<TimeRangeInput> | null | undefined;
};
export type KeywordInput = {
  caseSensitive?: boolean;
  fields: ReadonlyArray<KeywordField>;
  regexp?: boolean;
  text: string;
};
export type GenreInput = {
  lv1: number;
  lv2?: number | null | undefined;
};
export type TimeRangeInput = {
  durationMinutes: number;
  startMinute: number;
  weekdays: ReadonlyArray<number>;
};
export type WatchRuleEditorCreateMutation$variables = {
  input: WatchRuleInput;
};
export type WatchRuleEditorCreateMutation$data = {
  readonly createWatchRule: {
    readonly id: string;
  };
};
export type WatchRuleEditorCreateMutation = {
  response: WatchRuleEditorCreateMutation$data;
  variables: WatchRuleEditorCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "WatchRule",
    "kind": "LinkedField",
    "name": "createWatchRule",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "WatchRuleEditorCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "WatchRuleEditorCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "72b6ca9d50a24a6ca797f592bd66f2e9",
    "id": null,
    "metadata": {},
    "name": "WatchRuleEditorCreateMutation",
    "operationKind": "mutation",
    "text": "mutation WatchRuleEditorCreateMutation(\n  $input: WatchRuleInput!\n) {\n  createWatchRule(input: $input) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "6a1d6de24fdd7a8a39b3e36462a0a854";

export default node;
