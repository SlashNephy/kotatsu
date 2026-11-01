/**
 * @generated SignedSource<<2bc8c20bbb44ec2ec240370c0ddf37ff>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
import { FragmentRefs } from "relay-runtime";
export type WatchRuleIdQuery$variables = {
  id: string;
  skip: boolean;
};
export type WatchRuleIdQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly type: string;
  }>;
  readonly node?: {
    readonly __typename: "WatchRule";
    readonly id: string;
    readonly watchRule: {
      readonly " $fragmentSpreads": FragmentRefs<"WatchRuleEditor_watchRule">;
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
};
export type WatchRuleIdQuery = {
  response: WatchRuleIdQuery$data;
  variables: WatchRuleIdQuery$variables;
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
  "name": "displayName",
  "storageKey": null
},
v2 = {
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
    (v1/*:: as any*/)
  ],
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "services",
  "plural": true,
  "selections": [
    (v3/*:: as any*/),
    (v4/*:: as any*/),
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
        (v1/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v6 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v8 = [
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
v9 = [
  (v3/*:: as any*/)
],
v10 = [
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
    "name": "WatchRuleIdQuery",
    "selections": [
      (v2/*:: as any*/),
      (v5/*:: as any*/),
      {
        "condition": "skip",
        "kind": "Condition",
        "passingValue": false,
        "selections": [
          {
            "alias": null,
            "args": (v6/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v7/*:: as any*/),
              {
                "kind": "InlineFragment",
                "selections": [
                  (v3/*:: as any*/),
                  {
                    "fragment": {
                      "kind": "InlineFragment",
                      "selections": [
                        {
                          "args": null,
                          "kind": "FragmentSpread",
                          "name": "WatchRuleEditor_watchRule"
                        }
                      ],
                      "type": "WatchRule",
                      "abstractKey": null
                    },
                    "kind": "AliasedInlineFragmentSpread",
                    "name": "watchRule"
                  }
                ],
                "type": "WatchRule",
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
    "name": "WatchRuleIdQuery",
    "selections": [
      (v2/*:: as any*/),
      (v5/*:: as any*/),
      {
        "condition": "skip",
        "kind": "Condition",
        "passingValue": false,
        "selections": [
          {
            "alias": null,
            "args": (v6/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v7/*:: as any*/),
              (v3/*:: as any*/),
              {
                "kind": "InlineFragment",
                "selections": [
                  (v4/*:: as any*/),
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
                        "selections": (v8/*:: as any*/),
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
                        "selections": (v8/*:: as any*/),
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
                        "selections": (v9/*:: as any*/),
                        "storageKey": null
                      },
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Service",
                        "kind": "LinkedField",
                        "name": "excludeServices",
                        "plural": true,
                        "selections": (v9/*:: as any*/),
                        "storageKey": null
                      },
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "GenreCondition",
                        "kind": "LinkedField",
                        "name": "genres",
                        "plural": true,
                        "selections": (v10/*:: as any*/),
                        "storageKey": null
                      },
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "GenreCondition",
                        "kind": "LinkedField",
                        "name": "excludeGenres",
                        "plural": true,
                        "selections": (v10/*:: as any*/),
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
                      },
                      {
                        "alias": null,
                        "args": null,
                        "kind": "ScalarField",
                        "name": "marks",
                        "storageKey": null
                      },
                      {
                        "alias": null,
                        "args": null,
                        "kind": "ScalarField",
                        "name": "excludeMarks",
                        "storageKey": null
                      }
                    ],
                    "storageKey": null
                  }
                ],
                "type": "WatchRule",
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
    "cacheID": "4be8f8c2afac69e786d0d8e90afbe3d6",
    "id": null,
    "metadata": {},
    "name": "WatchRuleIdQuery",
    "operationKind": "query",
    "text": "query WatchRuleIdQuery(\n  $id: ID!\n  $skip: Boolean!\n) {\n  channelTypes {\n    type\n    displayName\n  }\n  services {\n    id\n    name\n    logoUrl\n    serviceId\n    channelType {\n      displayName\n    }\n  }\n  node(id: $id) @skip(if: $skip) {\n    __typename\n    ... on WatchRule {\n      id\n      ...WatchRuleEditor_watchRule\n    }\n    id\n  }\n}\n\nfragment WatchRuleEditor_watchRule on WatchRule {\n  id\n  name\n  enabled\n  conditions {\n    keywords {\n      text\n      regexp\n      caseSensitive\n      fields\n    }\n    keywordMatch\n    excludeKeywords {\n      text\n      regexp\n      caseSensitive\n      fields\n    }\n    channelTypes\n    services {\n      id\n    }\n    excludeServices {\n      id\n    }\n    genres {\n      lv1\n      lv2\n    }\n    excludeGenres {\n      lv1\n      lv2\n    }\n    timeRanges {\n      weekdays\n      startMinute\n      durationMinutes\n    }\n    minDurationSeconds\n    maxDurationSeconds\n    freeOnly\n    marks\n    excludeMarks\n  }\n}\n"
  }
};
})();

(node as any).hash = "a6ef8bf8a79acf022f83e7eaa8cdb4b7";

export default node;
