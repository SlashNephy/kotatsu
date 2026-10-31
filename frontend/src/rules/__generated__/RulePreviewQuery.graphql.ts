/**
 * @generated SignedSource<<cf082fccc13f1548739170fcfdb33d7c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type KeywordMatch = "ALL" | "ANY" | "%future added value";
export type RuleWarningKind = "UNFILTERABLE_REGEXP" | "%future added value";
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
  readonly ruleWarnings: ReadonlyArray<{
    readonly exclude: boolean;
    readonly index: number;
    readonly kind: RuleWarningKind;
  }>;
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
v1 = {
  "kind": "Variable",
  "name": "input",
  "variableName": "input"
},
v2 = {
  "alias": null,
  "args": [
    (v1/*:: as any*/)
  ],
  "concreteType": "RuleWarning",
  "kind": "LinkedField",
  "name": "ruleWarnings",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "kind",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "exclude",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "index",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v3 = [
  {
    "kind": "Literal",
    "name": "first",
    "value": 30
  },
  (v1/*:: as any*/)
],
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v8 = {
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
      (v2/*:: as any*/),
      {
        "alias": null,
        "args": (v3/*:: as any*/),
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
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v5/*:: as any*/),
                      (v7/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v8/*:: as any*/)
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
      (v2/*:: as any*/),
      {
        "alias": null,
        "args": (v3/*:: as any*/),
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
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v5/*:: as any*/),
                      (v7/*:: as any*/),
                      (v4/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v8/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "b6abbd621faa20ac63a7d2d3c0a1c9c7",
    "id": null,
    "metadata": {},
    "name": "RulePreviewQuery",
    "operationKind": "query",
    "text": "query RulePreviewQuery(\n  $input: RuleInput!\n) {\n  ruleWarnings(input: $input) {\n    kind\n    exclude\n    index\n  }\n  previewRule(input: $input, first: 30) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        service {\n          name\n          logoUrl\n          id\n        }\n      }\n    }\n    pageInfo {\n      hasNextPage\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "9115601e0a72f795520ea8bb5b930731";

export default node;
