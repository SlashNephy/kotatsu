/**
 * @generated SignedSource<<97cb65ba6f39e2b6aa492dbb73bd0c3d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type RuleInput = {
  avoidDuplicates?: boolean;
  conditions: ProgramConditionsInput;
  duplicateWindowDays?: number | null | undefined;
  enabled?: boolean;
  followEventRelay?: boolean;
  memo?: string;
  name: string;
  settings?: RecordingSettingsOverridesInput | null | undefined;
  validFrom?: string | null | undefined;
  validUntil?: string | null | undefined;
};
export type ProgramConditionsInput = {
  channelTypes?: ReadonlyArray<string> | null | undefined;
  excludeGenres?: ReadonlyArray<GenreInput> | null | undefined;
  excludeKeywords?: ReadonlyArray<KeywordInput> | null | undefined;
  freeOnly?: boolean | null | undefined;
  genres?: ReadonlyArray<GenreInput> | null | undefined;
  keywords?: ReadonlyArray<KeywordInput> | null | undefined;
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
export type RecordingSettingsOverridesInput = {
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
};
export type CreateRuleDialogMutation$variables = {
  input: RuleInput;
};
export type CreateRuleDialogMutation$data = {
  readonly createRule: {
    readonly id: string;
  };
};
export type CreateRuleDialogMutation = {
  response: CreateRuleDialogMutation$data;
  variables: CreateRuleDialogMutation$variables;
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
    "concreteType": "Rule",
    "kind": "LinkedField",
    "name": "createRule",
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
    "name": "CreateRuleDialogMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "CreateRuleDialogMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "112efef2bbc07ac92e02bbf95926923d",
    "id": null,
    "metadata": {},
    "name": "CreateRuleDialogMutation",
    "operationKind": "mutation",
    "text": "mutation CreateRuleDialogMutation(\n  $input: RuleInput!\n) {\n  createRule(input: $input) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "17d51cbcec5c0c51fb836e163f2d73c0";

export default node;
