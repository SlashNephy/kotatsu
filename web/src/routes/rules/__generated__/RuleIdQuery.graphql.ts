/**
 * @generated SignedSource<<28699988b65dab064f3b3f96117a6d98>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
import { FragmentRefs } from "relay-runtime";
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
    readonly id: string;
    readonly rule: {
      readonly " $fragmentSpreads": FragmentRefs<"RuleEditor_rule">;
    } | null | undefined;
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
    readonly logoUrl: string | null | undefined;
    readonly name: string;
    readonly serviceId: number;
  }>;
  readonly storagePoolNames: ReadonlyArray<string>;
  readonly systemSettings: {
    readonly recordingDefaults: {
      readonly poolId: string | null | undefined;
    };
  };
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
  "name": "storagePoolNames",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "poolId",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "SystemSettings",
  "kind": "LinkedField",
  "name": "systemSettings",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "concreteType": "RecordingSettings",
      "kind": "LinkedField",
      "name": "recordingDefaults",
      "plural": false,
      "selections": [
        (v5/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "services",
  "plural": true,
  "selections": [
    (v7/*:: as any*/),
    (v8/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "logoUrl",
      "storageKey": null
    },
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
v10 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v12 = [
  (v2/*:: as any*/),
  (v7/*:: as any*/)
],
v13 = [
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
v14 = [
  (v7/*:: as any*/)
],
v15 = [
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
      (v4/*:: as any*/),
      (v6/*:: as any*/),
      (v9/*:: as any*/),
      {
        "condition": "skip",
        "kind": "Condition",
        "passingValue": false,
        "selections": [
          {
            "alias": null,
            "args": (v10/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v11/*:: as any*/),
              {
                "kind": "InlineFragment",
                "selections": [
                  (v7/*:: as any*/),
                  {
                    "fragment": {
                      "kind": "InlineFragment",
                      "selections": [
                        {
                          "args": null,
                          "kind": "FragmentSpread",
                          "name": "RuleEditor_rule"
                        }
                      ],
                      "type": "Rule",
                      "abstractKey": null
                    },
                    "kind": "AliasedInlineFragmentSpread",
                    "name": "rule"
                  }
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
          (v7/*:: as any*/)
        ],
        "storageKey": null
      },
      (v3/*:: as any*/),
      (v4/*:: as any*/),
      (v6/*:: as any*/),
      (v9/*:: as any*/),
      {
        "condition": "skip",
        "kind": "Condition",
        "passingValue": false,
        "selections": [
          {
            "alias": null,
            "args": (v10/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v11/*:: as any*/),
              (v7/*:: as any*/),
              {
                "kind": "InlineFragment",
                "selections": [
                  {
                    "alias": null,
                    "args": null,
                    "kind": "ScalarField",
                    "name": "createdAt",
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "User",
                    "kind": "LinkedField",
                    "name": "createdBy",
                    "plural": false,
                    "selections": (v12/*:: as any*/),
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "kind": "ScalarField",
                    "name": "updatedAt",
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "User",
                    "kind": "LinkedField",
                    "name": "updatedBy",
                    "plural": false,
                    "selections": (v12/*:: as any*/),
                    "storageKey": null
                  },
                  (v8/*:: as any*/),
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
                        "selections": (v13/*:: as any*/),
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
                        "selections": (v13/*:: as any*/),
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
                        "selections": (v14/*:: as any*/),
                        "storageKey": null
                      },
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Service",
                        "kind": "LinkedField",
                        "name": "excludeServices",
                        "plural": true,
                        "selections": (v14/*:: as any*/),
                        "storageKey": null
                      },
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "GenreCondition",
                        "kind": "LinkedField",
                        "name": "genres",
                        "plural": true,
                        "selections": (v15/*:: as any*/),
                        "storageKey": null
                      },
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "GenreCondition",
                        "kind": "LinkedField",
                        "name": "excludeGenres",
                        "plural": true,
                        "selections": (v15/*:: as any*/),
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
                      (v5/*:: as any*/),
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
    "cacheID": "0fe507b10b2c36eac7d7cf3a8c795b4f",
    "id": null,
    "metadata": {},
    "name": "RuleIdQuery",
    "operationKind": "query",
    "text": "query RuleIdQuery(\n  $id: ID!\n  $skip: Boolean!\n) {\n  viewer {\n    role\n    id\n  }\n  channelTypes {\n    type\n    displayName\n  }\n  storagePoolNames\n  systemSettings {\n    recordingDefaults {\n      poolId\n    }\n  }\n  services {\n    id\n    name\n    logoUrl\n    serviceId\n    channelType {\n      displayName\n    }\n  }\n  node(id: $id) @skip(if: $skip) {\n    __typename\n    ... on Rule {\n      id\n      ...RuleEditor_rule\n    }\n    id\n  }\n}\n\nfragment RuleEditor_rule on Rule {\n  id\n  createdAt\n  createdBy {\n    displayName\n    id\n  }\n  updatedAt\n  updatedBy {\n    displayName\n    id\n  }\n  name\n  memo\n  enabled\n  conditions {\n    keywords {\n      text\n      regexp\n      caseSensitive\n      fields\n    }\n    keywordMatch\n    excludeKeywords {\n      text\n      regexp\n      caseSensitive\n      fields\n    }\n    channelTypes\n    services {\n      id\n    }\n    excludeServices {\n      id\n    }\n    genres {\n      lv1\n      lv2\n    }\n    excludeGenres {\n      lv1\n      lv2\n    }\n    timeRanges {\n      weekdays\n      startMinute\n      durationMinutes\n    }\n    minDurationSeconds\n    maxDurationSeconds\n    freeOnly\n  }\n  settings {\n    poolId\n    fileNameTemplate\n    startMarginSeconds\n    endMarginSeconds\n    priority\n    joinSegments\n  }\n  validFrom\n  validUntil\n  avoidDuplicates\n  duplicateWindowDays\n  followEventRelay\n}\n"
  }
};
})();

(node as any).hash = "4bce4f9011a00f132214e9b1a516c3e2";

export default node;
