/**
 * @generated SignedSource<<00c39da6902985fd2e9960db49e0dcce>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AssignmentState = "CONFLICT" | "OK" | "PARTIAL" | "PENDING" | "%future added value";
export type RecordingState = "COMPLETED" | "COMPLETED_WITH_ERRORS" | "FAILED" | "PARTIAL" | "RECORDING" | "%future added value";
export type DashboardQuery$variables = {
  from: string;
  nowEnd: string;
  to: string;
};
export type DashboardQuery$data = {
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly endAt: string;
      readonly id: string;
      readonly name: string;
      readonly startAt: string;
    }>;
    readonly service: {
      readonly id: string;
      readonly name: string;
    };
  }>;
  readonly recordings: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly id: string;
        readonly name: string;
        readonly recordedFrom: string;
        readonly service: {
          readonly name: string;
        };
        readonly state: RecordingState;
        readonly thumbnailUrl: string | null | undefined;
      };
    }>;
  };
  readonly troubles: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly id: string;
      };
    }>;
    readonly pageInfo: {
      readonly hasNextPage: boolean;
    };
  };
  readonly tunerServerHealth: {
    readonly degraded: boolean;
    readonly degradedSince: string | null | undefined;
  };
  readonly upcoming: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly assignment: {
          readonly state: AssignmentState;
        };
        readonly endAt: string;
        readonly id: string;
        readonly name: string;
        readonly service: {
          readonly name: string;
        };
        readonly startAt: string;
      };
    }>;
  };
};
export type DashboardQuery = {
  response: DashboardQuery$data;
  variables: DashboardQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "from"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "nowEnd"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "to"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "concreteType": "TunerServerHealth",
  "kind": "LinkedField",
  "name": "tunerServerHealth",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "degraded",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "degradedSince",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v2 = {
  "kind": "Variable",
  "name": "from",
  "variableName": "from"
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
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": [
    (v2/*:: as any*/),
    {
      "kind": "Variable",
      "name": "to",
      "variableName": "nowEnd"
    }
  ],
  "concreteType": "GuideRow",
  "kind": "LinkedField",
  "name": "guide",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "concreteType": "Service",
      "kind": "LinkedField",
      "name": "service",
      "plural": false,
      "selections": [
        (v3/*:: as any*/),
        (v4/*:: as any*/)
      ],
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "Program",
      "kind": "LinkedField",
      "name": "programs",
      "plural": true,
      "selections": [
        (v3/*:: as any*/),
        (v4/*:: as any*/),
        (v5/*:: as any*/),
        (v6/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v8 = [
  {
    "kind": "Literal",
    "name": "first",
    "value": 6
  }
],
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v4/*:: as any*/)
  ],
  "storageKey": null
},
v13 = {
  "kind": "Literal",
  "name": "enabled",
  "value": true
},
v14 = {
  "kind": "Variable",
  "name": "to",
  "variableName": "to"
},
v15 = [
  (v13/*:: as any*/),
  {
    "kind": "Literal",
    "name": "first",
    "value": 5
  },
  (v2/*:: as any*/),
  (v14/*:: as any*/)
],
v16 = {
  "alias": null,
  "args": null,
  "concreteType": "ReservationAssignment",
  "kind": "LinkedField",
  "name": "assignment",
  "plural": false,
  "selections": [
    (v9/*:: as any*/)
  ],
  "storageKey": null
},
v17 = {
  "alias": "troubles",
  "args": [
    (v13/*:: as any*/),
    {
      "kind": "Literal",
      "name": "first",
      "value": 100
    },
    (v2/*:: as any*/),
    {
      "kind": "Literal",
      "name": "states",
      "value": [
        "CONFLICT",
        "PARTIAL"
      ]
    },
    (v14/*:: as any*/)
  ],
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
            (v3/*:: as any*/)
          ],
          "storageKey": null
        }
      ],
      "storageKey": null
    },
    {
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
    }
  ],
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v4/*:: as any*/),
    (v3/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "DashboardQuery",
    "selections": [
      (v1/*:: as any*/),
      (v7/*:: as any*/),
      {
        "alias": null,
        "args": (v8/*:: as any*/),
        "concreteType": "RecordingConnection",
        "kind": "LinkedField",
        "name": "recordings",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "RecordingEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Recording",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  (v9/*:: as any*/),
                  (v10/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": "recordings(first:6)"
      },
      {
        "alias": "upcoming",
        "args": (v15/*:: as any*/),
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
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  (v12/*:: as any*/),
                  (v16/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v17/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "DashboardQuery",
    "selections": [
      (v1/*:: as any*/),
      (v7/*:: as any*/),
      {
        "alias": null,
        "args": (v8/*:: as any*/),
        "concreteType": "RecordingConnection",
        "kind": "LinkedField",
        "name": "recordings",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "RecordingEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Recording",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  (v9/*:: as any*/),
                  (v10/*:: as any*/),
                  (v11/*:: as any*/),
                  (v18/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": "recordings(first:6)"
      },
      {
        "alias": "upcoming",
        "args": (v15/*:: as any*/),
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
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  (v18/*:: as any*/),
                  (v16/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v17/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "3122ccb4137d54845204ab6438134e3c",
    "id": null,
    "metadata": {},
    "name": "DashboardQuery",
    "operationKind": "query",
    "text": "query DashboardQuery(\n  $from: Time!\n  $nowEnd: Time!\n  $to: Time!\n) {\n  tunerServerHealth {\n    degraded\n    degradedSince\n  }\n  guide(from: $from, to: $nowEnd) {\n    service {\n      id\n      name\n    }\n    programs {\n      id\n      name\n      startAt\n      endAt\n    }\n  }\n  recordings(first: 6) {\n    edges {\n      node {\n        id\n        name\n        state\n        recordedFrom\n        thumbnailUrl\n        service {\n          name\n          id\n        }\n      }\n    }\n  }\n  upcoming: reservations(from: $from, to: $to, enabled: true, first: 5) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        service {\n          name\n          id\n        }\n        assignment {\n          state\n        }\n      }\n    }\n  }\n  troubles: reservations(from: $from, to: $to, states: [CONFLICT, PARTIAL], enabled: true, first: 100) {\n    edges {\n      node {\n        id\n      }\n    }\n    pageInfo {\n      hasNextPage\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "03c78c72bdc0f281d0e746bdf2d039cc";

export default node;
