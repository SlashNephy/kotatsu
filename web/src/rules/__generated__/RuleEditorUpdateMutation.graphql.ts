/**
 * @generated SignedSource<<09f5da6967038acf2c256228c8c4ed81>>
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
export type RuleEditorUpdateMutation$variables = {
  id: string;
  input: RuleInput;
};
export type RuleEditorUpdateMutation$data = {
  readonly updateRule: {
    readonly enabled: boolean;
    readonly id: string;
    readonly name: string;
  };
};
export type RuleEditorUpdateMutation = {
  response: RuleEditorUpdateMutation$data;
  variables: RuleEditorUpdateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
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
        "name": "id",
        "variableName": "id"
      },
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "Rule",
    "kind": "LinkedField",
    "name": "updateRule",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "name",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "enabled",
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
    "name": "RuleEditorUpdateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleEditorUpdateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "472ab10c768a221cb19d1d44f4d5552c",
    "id": null,
    "metadata": {},
    "name": "RuleEditorUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation RuleEditorUpdateMutation(\n  $id: ID!\n  $input: RuleInput!\n) {\n  updateRule(id: $id, input: $input) {\n    id\n    name\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "ed7f0328ffb36a5f6b11759893228c91";

export default node;
