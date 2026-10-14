/**
 * @generated SignedSource<<29d2d503b92e77fe8bd86f51b1ac0571>>
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
  recordings: number;
  to: string;
};
export type DashboardQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly type: string;
  }>;
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly endAt: string;
      readonly id: string;
      readonly name: string;
      readonly sharedFrom: {
        readonly id: string;
      } | null | undefined;
      readonly startAt: string;
    }>;
    readonly service: {
      readonly channelType: {
        readonly type: string;
      };
      readonly id: string;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
  }>;
  readonly recordings: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly endAt: string;
        readonly id: string;
        readonly name: string;
        readonly recordedFrom: string;
        readonly service: {
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
        readonly startAt: string;
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
          readonly logoUrl: string | null | undefined;
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
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "from"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "nowEnd"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "recordings"
},
v3 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "to"
},
v4 = {
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
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "type",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "ChannelType",
  "kind": "LinkedField",
  "name": "channelTypes",
  "plural": true,
  "selections": [
    (v5/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "displayName",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v7 = {
  "kind": "Variable",
  "name": "from",
  "variableName": "from"
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v13 = [
  (v8/*:: as any*/)
],
v14 = {
  "alias": null,
  "args": [
    (v7/*:: as any*/),
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
        (v8/*:: as any*/),
        (v9/*:: as any*/),
        (v10/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "concreteType": "ChannelType",
          "kind": "LinkedField",
          "name": "channelType",
          "plural": false,
          "selections": [
            (v5/*:: as any*/)
          ],
          "storageKey": null
        }
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
        (v8/*:: as any*/),
        (v9/*:: as any*/),
        (v11/*:: as any*/),
        (v12/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "concreteType": "Service",
          "kind": "LinkedField",
          "name": "sharedFrom",
          "plural": false,
          "selections": (v13/*:: as any*/),
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v15 = [
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "recordings"
  }
],
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v9/*:: as any*/),
    (v10/*:: as any*/)
  ],
  "storageKey": null
},
v20 = {
  "kind": "Literal",
  "name": "enabled",
  "value": true
},
v21 = {
  "kind": "Variable",
  "name": "to",
  "variableName": "to"
},
v22 = [
  (v20/*:: as any*/),
  {
    "kind": "Literal",
    "name": "first",
    "value": 5
  },
  (v7/*:: as any*/),
  (v21/*:: as any*/)
],
v23 = {
  "alias": null,
  "args": null,
  "concreteType": "ReservationAssignment",
  "kind": "LinkedField",
  "name": "assignment",
  "plural": false,
  "selections": [
    (v16/*:: as any*/)
  ],
  "storageKey": null
},
v24 = {
  "alias": "troubles",
  "args": [
    (v20/*:: as any*/),
    {
      "kind": "Literal",
      "name": "first",
      "value": 100
    },
    (v7/*:: as any*/),
    {
      "kind": "Literal",
      "name": "states",
      "value": [
        "CONFLICT",
        "PARTIAL"
      ]
    },
    (v21/*:: as any*/)
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
          "selections": (v13/*:: as any*/),
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
v25 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v9/*:: as any*/),
    (v10/*:: as any*/),
    (v8/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      (v3/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "DashboardQuery",
    "selections": [
      (v4/*:: as any*/),
      (v6/*:: as any*/),
      (v14/*:: as any*/),
      {
        "alias": null,
        "args": (v15/*:: as any*/),
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
                  (v8/*:: as any*/),
                  (v9/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/),
                  (v16/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  (v19/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": "upcoming",
        "args": (v22/*:: as any*/),
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
                  (v8/*:: as any*/),
                  (v9/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/),
                  (v19/*:: as any*/),
                  (v23/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v24/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v3/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "DashboardQuery",
    "selections": [
      (v4/*:: as any*/),
      (v6/*:: as any*/),
      (v14/*:: as any*/),
      {
        "alias": null,
        "args": (v15/*:: as any*/),
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
                  (v8/*:: as any*/),
                  (v9/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/),
                  (v16/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  (v25/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": "upcoming",
        "args": (v22/*:: as any*/),
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
                  (v8/*:: as any*/),
                  (v9/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/),
                  (v25/*:: as any*/),
                  (v23/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v24/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "1cee4fa979081d2f99ba6e7a90f3b1ea",
    "id": null,
    "metadata": {},
    "name": "DashboardQuery",
    "operationKind": "query",
    "text": "query DashboardQuery(\n  $from: Time!\n  $nowEnd: Time!\n  $to: Time!\n  $recordings: Int!\n) {\n  tunerServerHealth {\n    degraded\n    degradedSince\n  }\n  channelTypes {\n    type\n    displayName\n  }\n  guide(from: $from, to: $nowEnd) {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        type\n      }\n    }\n    programs {\n      id\n      name\n      startAt\n      endAt\n      sharedFrom {\n        id\n      }\n    }\n  }\n  recordings(first: $recordings) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        state\n        recordedFrom\n        thumbnailUrl\n        service {\n          name\n          logoUrl\n          id\n        }\n      }\n    }\n  }\n  upcoming: reservations(from: $from, to: $to, enabled: true, first: 5) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        service {\n          name\n          logoUrl\n          id\n        }\n        assignment {\n          state\n        }\n      }\n    }\n  }\n  troubles: reservations(from: $from, to: $to, states: [CONFLICT, PARTIAL], enabled: true, first: 100) {\n    edges {\n      node {\n        id\n      }\n    }\n    pageInfo {\n      hasNextPage\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "7c7e95768ad7e48b39ee6166c33936fd";

export default node;
