/**
 * @generated SignedSource<<71ea7996bf409da60d56ca06b0e41c7b>>
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
export type RulePreviewQuery$variables = {
  input: RuleInput;
};
export type RulePreviewQuery$data = {
  readonly previewRule: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly id: string;
        readonly name: string;
        readonly service: {
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
        readonly startAt: string;
      };
    }>;
    readonly pageInfo: {
      readonly hasNextPage: boolean;
    };
  };
};
export type RulePreviewQuery = {
  response: RulePreviewQuery$data;
  variables: RulePreviewQuery$variables;
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
    "kind": "Literal",
    "name": "first",
    "value": 30
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
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "PageInfo",
  "kind": "LinkedField",
  "name": "pageInfo",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "hasNextPage",
      "storageKey": null
    }
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RulePreviewQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "ProgramConnection",
        "kind": "LinkedField",
        "name": "previewRule",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "ProgramEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Program",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v2/*:: as any*/),
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v3/*:: as any*/),
                      (v5/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v6/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RulePreviewQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "ProgramConnection",
        "kind": "LinkedField",
        "name": "previewRule",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "ProgramEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Program",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v2/*:: as any*/),
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v3/*:: as any*/),
                      (v5/*:: as any*/),
                      (v2/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v6/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "56bc9f40e2916a479cea885a7d6f88d9",
    "id": null,
    "metadata": {},
    "name": "RulePreviewQuery",
    "operationKind": "query",
    "text": "query RulePreviewQuery(\n  $input: RuleInput!\n) {\n  previewRule(input: $input, first: 30) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        service {\n          name\n          logoUrl\n          id\n        }\n      }\n    }\n    pageInfo {\n      hasNextPage\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "97e3fdddfe3d0dd6cd29ead8839582ea";

export default node;
