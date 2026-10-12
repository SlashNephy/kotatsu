/**
 * @generated SignedSource<<d253a0bb7b6fadcdcfa627fc4c72ee08>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type KeywordField = "DESCRIPTION" | "EXTENDED" | "NAME" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type RuleIdQuery$variables = {
  id: string;
  skip: boolean;
};
export type RuleIdQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly type: string;
  }>;
  readonly node?: {
    readonly __typename: "Rule";
    readonly avoidDuplicates: boolean;
    readonly conditions: {
      readonly channelTypes: ReadonlyArray<string>;
      readonly excludeGenres: ReadonlyArray<{
        readonly lv1: number;
        readonly lv2: number | null | undefined;
      }>;
      readonly excludeKeywords: ReadonlyArray<{
        readonly caseSensitive: boolean;
        readonly fields: ReadonlyArray<KeywordField>;
        readonly regexp: boolean;
        readonly text: string;
      }>;
      readonly freeOnly: boolean;
      readonly genres: ReadonlyArray<{
        readonly lv1: number;
        readonly lv2: number | null | undefined;
      }>;
      readonly keywords: ReadonlyArray<{
        readonly caseSensitive: boolean;
        readonly fields: ReadonlyArray<KeywordField>;
        readonly regexp: boolean;
        readonly text: string;
      }>;
      readonly maxDurationSeconds: number | null | undefined;
      readonly minDurationSeconds: number | null | undefined;
      readonly services: ReadonlyArray<{
        readonly id: string;
      }>;
      readonly timeRanges: ReadonlyArray<{
        readonly durationMinutes: number;
        readonly startMinute: number;
        readonly weekdays: ReadonlyArray<number>;
      }>;
    };
    readonly duplicateWindowDays: number | null | undefined;
    readonly enabled: boolean;
    readonly followEventRelay: boolean;
    readonly id: string;
    readonly memo: string;
    readonly name: string;
    readonly settings: {
      readonly endMarginSeconds: number | null | undefined;
      readonly fileNameTemplate: string | null | undefined;
      readonly joinSegments: boolean | null | undefined;
      readonly poolId: string | null | undefined;
      readonly priority: number | null | undefined;
      readonly startMarginSeconds: number | null | undefined;
    };
    readonly validFrom: string | null | undefined;
    readonly validUntil: string | null | undefined;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly displayName: string;
    };
    readonly id: string;
    readonly name: string;
    readonly serviceId: number;
  }>;
  readonly viewer: {
    readonly role: Role;
  };
};
export type RuleIdQuery = {
  response: RuleIdQuery$data;
  variables: RuleIdQuery$variables;
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
    "name": "skip"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "concreteType": "ChannelType",
  "kind": "LinkedField",
  "name": "channelTypes",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "type",
      "storageKey": null
    },
    (v2/*:: as any*/)
  ],
  "storageKey": null
},
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
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "services",
  "plural": true,
  "selections": [
    (v4/*:: as any*/),
    (v5/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "serviceId",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "ChannelType",
      "kind": "LinkedField",
      "name": "channelType",
      "plural": false,
      "selections": [
        (v2/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v7 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "memo",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "enabled",
  "storageKey": null
},
v11 = [
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
v12 = [
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
],
v13 = {
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
      "selections": (v11/*:: as any*/),
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "Keyword",
      "kind": "LinkedField",
      "name": "excludeKeywords",
      "plural": true,
      "selections": (v11/*:: as any*/),
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
        (v4/*:: as any*/)
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
      "selections": (v12/*:: as any*/),
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "GenreCondition",
      "kind": "LinkedField",
      "name": "excludeGenres",
      "plural": true,
      "selections": (v12/*:: as any*/),
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
v14 = {
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
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "validFrom",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "validUntil",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "avoidDuplicates",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "duplicateWindowDays",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "followEventRelay",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RuleIdQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v1/*:: as any*/)
        ],
        "storageKey": null
      },
      (v3/*:: as any*/),
      (v6/*:: as any*/),
      {
        "condition": "skip",
        "kind": "Condition",
        "passingValue": false,
        "selections": [
          {
            "alias": null,
            "args": (v7/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v8/*:: as any*/),
              {
                "kind": "InlineFragment",
                "selections": [
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v9/*:: as any*/),
                  (v10/*:: as any*/),
                  (v13/*:: as any*/),
                  (v14/*:: as any*/),
                  (v15/*:: as any*/),
                  (v16/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  (v19/*:: as any*/)
                ],
                "type": "Rule",
                "abstractKey": null
              }
            ],
            "storageKey": null
          }
        ]
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RuleIdQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          (v4/*:: as any*/)
        ],
        "storageKey": null
      },
      (v3/*:: as any*/),
      (v6/*:: as any*/),
      {
        "condition": "skip",
        "kind": "Condition",
        "passingValue": false,
        "selections": [
          {
            "alias": null,
            "args": (v7/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v8/*:: as any*/),
              (v4/*:: as any*/),
              {
                "kind": "InlineFragment",
                "selections": [
                  (v5/*:: as any*/),
                  (v9/*:: as any*/),
                  (v10/*:: as any*/),
                  (v13/*:: as any*/),
                  (v14/*:: as any*/),
                  (v15/*:: as any*/),
                  (v16/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  (v19/*:: as any*/)
                ],
                "type": "Rule",
                "abstractKey": null
              }
            ],
            "storageKey": null
          }
        ]
      }
    ]
  },
  "params": {
    "cacheID": "ae71745732edefc785bfaf6767cd1029",
    "id": null,
    "metadata": {},
    "name": "RuleIdQuery",
    "operationKind": "query",
    "text": "query RuleIdQuery(\n  $id: ID!\n  $skip: Boolean!\n) {\n  viewer {\n    role\n    id\n  }\n  channelTypes {\n    type\n    displayName\n  }\n  services {\n    id\n    name\n    serviceId\n    channelType {\n      displayName\n    }\n  }\n  node(id: $id) @skip(if: $skip) {\n    __typename\n    ... on Rule {\n      id\n      name\n      memo\n      enabled\n      conditions {\n        keywords {\n          text\n          regexp\n          caseSensitive\n          fields\n        }\n        excludeKeywords {\n          text\n          regexp\n          caseSensitive\n          fields\n        }\n        channelTypes\n        services {\n          id\n        }\n        genres {\n          lv1\n          lv2\n        }\n        excludeGenres {\n          lv1\n          lv2\n        }\n        timeRanges {\n          weekdays\n          startMinute\n          durationMinutes\n        }\n        minDurationSeconds\n        maxDurationSeconds\n        freeOnly\n      }\n      settings {\n        poolId\n        fileNameTemplate\n        startMarginSeconds\n        endMarginSeconds\n        priority\n        joinSegments\n      }\n      validFrom\n      validUntil\n      avoidDuplicates\n      duplicateWindowDays\n      followEventRelay\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "5faa158b626d1b9b34f9a5de2e93c5ae";

export default node;
