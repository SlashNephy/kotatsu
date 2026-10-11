/**
 * @generated SignedSource<<89dd4b233369b13718a3233f93b066f7>>
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
        readonly enabled: boolean;
        readonly id: string;
      };
    }>;
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
        readonly enabled: boolean;
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
  "kind": "Variable",
  "name": "to",
  "variableName": "to"
},
v14 = [
  {
    "kind": "Literal",
    "name": "first",
    "value": 5
  },
  (v2/*:: as any*/),
  (v13/*:: as any*/)
],
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "enabled",
  "storageKey": null
},
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
    (v13/*:: as any*/)
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
            (v3/*:: as any*/),
            (v15/*:: as any*/)
          ],
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
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  (v15/*:: as any*/),
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
                  (v3/*:: as any*/),
                  (v4/*:: as any*/),
                  (v5/*:: as any*/),
                  (v6/*:: as any*/),
                  (v15/*:: as any*/),
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
    "cacheID": "953add4aa070d2397fa44e318730a8ce",
    "id": null,
    "metadata": {},
    "name": "DashboardQuery",
    "operationKind": "query",
    "text": "query DashboardQuery(\n  $from: Time!\n  $nowEnd: Time!\n  $to: Time!\n) {\n  tunerServerHealth {\n    degraded\n    degradedSince\n  }\n  guide(from: $from, to: $nowEnd) {\n    service {\n      id\n      name\n    }\n    programs {\n      id\n      name\n      startAt\n      endAt\n    }\n  }\n  recordings(first: 6) {\n    edges {\n      node {\n        id\n        name\n        state\n        recordedFrom\n        thumbnailUrl\n        service {\n          name\n          id\n        }\n      }\n    }\n  }\n  upcoming: reservations(from: $from, to: $to, first: 5) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        enabled\n        service {\n          name\n          id\n        }\n        assignment {\n          state\n        }\n      }\n    }\n  }\n  troubles: reservations(from: $from, to: $to, states: [CONFLICT, PARTIAL], first: 100) {\n    edges {\n      node {\n        id\n        enabled\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "927ad60cf92022e4c7ee7159a35c46dc";

export default node;
