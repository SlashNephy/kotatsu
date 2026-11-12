/**
 * @generated SignedSource<<ced11ff8259c15bc03832f1aa0f390bb>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
import { FragmentRefs } from "relay-runtime";
export type AssignmentState = "CONFLICT" | "OK" | "PARTIAL" | "PENDING" | "%future added value";
export type ReservationDisabledReason = "RECORDED_EPISODE" | "%future added value";
export type ReservationKind = "PROGRAM" | "RULE" | "TIME" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type reservationsQuery$variables = {
  after?: string | null | undefined;
  first: number;
  from: string;
  rule?: string | null | undefined;
  ruleId: string;
  states?: ReadonlyArray<AssignmentState> | null | undefined;
  to: string;
  withRule: boolean;
};
export type reservationsQuery$data = {
  readonly reservations: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly assignment: {
          readonly plannedEndAt: string | null | undefined;
          readonly plannedStartAt: string | null | undefined;
          readonly state: AssignmentState;
          readonly tuner: number | null | undefined;
          readonly " $fragmentSpreads": FragmentRefs<"ReservationPeers_assignment">;
        };
        readonly disabledReason: ReservationDisabledReason | null | undefined;
        readonly disabledService: {
          readonly name: string;
        } | null | undefined;
        readonly enabled: boolean;
        readonly endAt: string;
        readonly id: string;
        readonly kind: ReservationKind;
        readonly name: string;
        readonly service: {
          readonly channelType: {
            readonly type: string;
          };
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
        readonly startAt: string;
      };
    }>;
    readonly pageInfo: {
      readonly endCursor: string | null | undefined;
      readonly hasNextPage: boolean;
    };
  };
  readonly ruleFilter?: {
    readonly __typename: "Rule";
    readonly name: string;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly viewer: {
    readonly role: Role;
  };
};
export type reservationsQuery = {
  response: reservationsQuery$data;
  variables: reservationsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "after"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "first"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "from"
},
v3 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "rule"
},
v4 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "ruleId"
},
v5 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "states"
},
v6 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "to"
},
v7 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "withRule"
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v9 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "ruleId"
  }
],
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v12 = [
  (v11/*:: as any*/)
],
v13 = {
  "kind": "InlineFragment",
  "selections": (v12/*:: as any*/),
  "type": "Rule",
  "abstractKey": null
},
v14 = [
  {
    "kind": "Variable",
    "name": "after",
    "variableName": "after"
  },
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "first"
  },
  {
    "kind": "Variable",
    "name": "from",
    "variableName": "from"
  },
  {
    "kind": "Variable",
    "name": "rule",
    "variableName": "rule"
  },
  {
    "kind": "Variable",
    "name": "states",
    "variableName": "states"
  },
  {
    "kind": "Variable",
    "name": "to",
    "variableName": "to"
  }
],
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "enabled",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "disabledReason",
  "storageKey": null
},
v21 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v22 = {
  "alias": null,
  "args": null,
  "concreteType": "ChannelType",
  "kind": "LinkedField",
  "name": "channelType",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "type",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v23 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v24 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "tuner",
  "storageKey": null
},
v25 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "plannedStartAt",
  "storageKey": null
},
v26 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "plannedEndAt",
  "storageKey": null
},
v27 = {
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
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "endCursor",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v28 = [
  (v11/*:: as any*/),
  (v15/*:: as any*/)
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      (v3/*:: as any*/),
      (v4/*:: as any*/),
      (v5/*:: as any*/),
      (v6/*:: as any*/),
      (v7/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "reservationsQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v8/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "condition": "withRule",
        "kind": "Condition",
        "passingValue": true,
        "selections": [
          {
            "alias": "ruleFilter",
            "args": (v9/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v10/*:: as any*/),
              (v13/*:: as any*/)
            ],
            "storageKey": null
          }
        ]
      },
      {
        "alias": null,
        "args": (v14/*:: as any*/),
        "concreteType": "ReservationConnection",
        "kind": "LinkedField",
        "name": "reservations",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "ReservationEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Reservation",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v15/*:: as any*/),
                  (v16/*:: as any*/),
                  (v11/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  (v19/*:: as any*/),
                  (v20/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "disabledService",
                    "plural": false,
                    "selections": (v12/*:: as any*/),
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v11/*:: as any*/),
                      (v21/*:: as any*/),
                      (v22/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "ReservationAssignment",
                    "kind": "LinkedField",
                    "name": "assignment",
                    "plural": false,
                    "selections": [
                      (v23/*:: as any*/),
                      (v24/*:: as any*/),
                      (v25/*:: as any*/),
                      (v26/*:: as any*/),
                      {
                        "args": null,
                        "kind": "FragmentSpread",
                        "name": "ReservationPeers_assignment"
                      }
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v27/*:: as any*/)
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v2/*:: as any*/),
      (v6/*:: as any*/),
      (v5/*:: as any*/),
      (v3/*:: as any*/),
      (v4/*:: as any*/),
      (v7/*:: as any*/),
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "reservationsQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v8/*:: as any*/),
          (v15/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "condition": "withRule",
        "kind": "Condition",
        "passingValue": true,
        "selections": [
          {
            "alias": "ruleFilter",
            "args": (v9/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v10/*:: as any*/),
              (v13/*:: as any*/),
              (v15/*:: as any*/)
            ],
            "storageKey": null
          }
        ]
      },
      {
        "alias": null,
        "args": (v14/*:: as any*/),
        "concreteType": "ReservationConnection",
        "kind": "LinkedField",
        "name": "reservations",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "ReservationEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Reservation",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v15/*:: as any*/),
                  (v16/*:: as any*/),
                  (v11/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  (v19/*:: as any*/),
                  (v20/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "disabledService",
                    "plural": false,
                    "selections": (v28/*:: as any*/),
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v11/*:: as any*/),
                      (v21/*:: as any*/),
                      (v22/*:: as any*/),
                      (v15/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "ReservationAssignment",
                    "kind": "LinkedField",
                    "name": "assignment",
                    "plural": false,
                    "selections": [
                      (v23/*:: as any*/),
                      (v24/*:: as any*/),
                      (v25/*:: as any*/),
                      (v26/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Reservation",
                        "kind": "LinkedField",
                        "name": "peers",
                        "plural": true,
                        "selections": [
                          (v15/*:: as any*/),
                          (v11/*:: as any*/),
                          {
                            "alias": null,
                            "args": null,
                            "concreteType": "Service",
                            "kind": "LinkedField",
                            "name": "service",
                            "plural": false,
                            "selections": (v28/*:: as any*/),
                            "storageKey": null
                          }
                        ],
                        "storageKey": null
                      }
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          (v27/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "1a85a0325efce3fd70eb11e47c9d5b07",
    "id": null,
    "metadata": {},
    "name": "reservationsQuery",
    "operationKind": "query",
    "text": "query reservationsQuery(\n  $from: Time!\n  $to: Time!\n  $states: [AssignmentState!]\n  $rule: ID\n  $ruleId: ID!\n  $withRule: Boolean!\n  $first: Int!\n  $after: String\n) {\n  viewer {\n    role\n    id\n  }\n  ruleFilter: node(id: $ruleId) @include(if: $withRule) {\n    __typename\n    ... on Rule {\n      name\n    }\n    id\n  }\n  reservations(from: $from, to: $to, states: $states, rule: $rule, first: $first, after: $after) {\n    edges {\n      node {\n        id\n        kind\n        name\n        startAt\n        endAt\n        enabled\n        disabledReason\n        disabledService {\n          name\n          id\n        }\n        service {\n          name\n          logoUrl\n          channelType {\n            type\n          }\n          id\n        }\n        assignment {\n          state\n          tuner\n          plannedStartAt\n          plannedEndAt\n          ...ReservationPeers_assignment\n        }\n      }\n    }\n    pageInfo {\n      hasNextPage\n      endCursor\n    }\n  }\n}\n\nfragment ReservationPeers_assignment on ReservationAssignment {\n  peers {\n    id\n    name\n    service {\n      name\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "598cdad5ac6671827c570b80b2c2d941";

export default node;
