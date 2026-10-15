/**
 * @generated SignedSource<<f5d3702a87905cb8214b066b18af44dc>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
import { FragmentRefs } from "relay-runtime";
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type KeywordMatch = "ALL" | "ANY" | "%future added value";
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
  keywordMatch?: KeywordMatch;
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
    readonly " $fragmentSpreads": FragmentRefs<"RuleEditor_rule">;
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
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "text",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "regexp",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "caseSensitive",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "fields",
    "storageKey": null
  }
],
v4 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "lv1",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "lv2",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RuleEditorUpdateMutation",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "Rule",
        "kind": "LinkedField",
        "name": "updateRule",
        "plural": false,
        "selections": [
          {
            "args": null,
            "kind": "FragmentSpread",
            "name": "RuleEditor_rule"
          }
        ],
        "storageKey": null
      }
    ],
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleEditorUpdateMutation",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "Rule",
        "kind": "LinkedField",
        "name": "updateRule",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
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
            "name": "memo",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "enabled",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "ProgramConditions",
            "kind": "LinkedField",
            "name": "conditions",
            "plural": false,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Keyword",
                "kind": "LinkedField",
                "name": "keywords",
                "plural": true,
                "selections": (v3/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "keywordMatch",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "Keyword",
                "kind": "LinkedField",
                "name": "excludeKeywords",
                "plural": true,
                "selections": (v3/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "channelTypes",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "services",
                "plural": true,
                "selections": [
                  (v2/*:: as any*/)
                ],
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "GenreCondition",
                "kind": "LinkedField",
                "name": "genres",
                "plural": true,
                "selections": (v4/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "GenreCondition",
                "kind": "LinkedField",
                "name": "excludeGenres",
                "plural": true,
                "selections": (v4/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "TimeRange",
                "kind": "LinkedField",
                "name": "timeRanges",
                "plural": true,
                "selections": [
                  {
                    "alias": null,
                    "args": null,
                    "kind": "ScalarField",
                    "name": "weekdays",
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "kind": "ScalarField",
                    "name": "startMinute",
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "kind": "ScalarField",
                    "name": "durationMinutes",
                    "storageKey": null
                  }
                ],
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "minDurationSeconds",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "maxDurationSeconds",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "freeOnly",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "RecordingSettingsOverrides",
            "kind": "LinkedField",
            "name": "settings",
            "plural": false,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "poolId",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "fileNameTemplate",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "startMarginSeconds",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "endMarginSeconds",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "priority",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "joinSegments",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "validFrom",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "validUntil",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "avoidDuplicates",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "duplicateWindowDays",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "followEventRelay",
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "6681b4c4e392d894ef5b22363f8f664b",
    "id": null,
    "metadata": {},
    "name": "RuleEditorUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation RuleEditorUpdateMutation(\n  $id: ID!\n  $input: RuleInput!\n) {\n  updateRule(id: $id, input: $input) {\n    ...RuleEditor_rule\n    id\n  }\n}\n\nfragment RuleEditor_rule on Rule {\n  id\n  name\n  memo\n  enabled\n  conditions {\n    keywords {\n      text\n      regexp\n      caseSensitive\n      fields\n    }\n    keywordMatch\n    excludeKeywords {\n      text\n      regexp\n      caseSensitive\n      fields\n    }\n    channelTypes\n    services {\n      id\n    }\n    genres {\n      lv1\n      lv2\n    }\n    excludeGenres {\n      lv1\n      lv2\n    }\n    timeRanges {\n      weekdays\n      startMinute\n      durationMinutes\n    }\n    minDurationSeconds\n    maxDurationSeconds\n    freeOnly\n  }\n  settings {\n    poolId\n    fileNameTemplate\n    startMarginSeconds\n    endMarginSeconds\n    priority\n    joinSegments\n  }\n  validFrom\n  validUntil\n  avoidDuplicates\n  duplicateWindowDays\n  followEventRelay\n}\n"
  }
};
})();

(node as any).hash = "444b3efd87cfbe761f68732cd01268d0";

export default node;
