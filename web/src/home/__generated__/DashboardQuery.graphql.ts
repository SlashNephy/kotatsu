/**
 * @generated SignedSource<<37a19a0494bf05a26b2ca0bac6baff71>>
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
  upcoming: number;
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
        readonly previewUrl: string | null | undefined;
        readonly recordedFrom: string;
        readonly recordedUntil: string | null | undefined;
        readonly service: {
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
        readonly startAt: string;
        readonly state: RecordingState;
        readonly thumbnailUrl: string | null | undefined;
        readonly watchPositionMs: number | null | undefined;
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
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "upcoming"
},
v5 = {
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
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "type",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "concreteType": "ChannelType",
  "kind": "LinkedField",
  "name": "channelTypes",
  "plural": true,
  "selections": [
    (v6/*:: as any*/),
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
v8 = {
  "kind": "Variable",
  "name": "from",
  "variableName": "from"
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v14 = [
  (v9/*:: as any*/)
],
v15 = {
  "alias": null,
  "args": [
    (v8/*:: as any*/),
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
        (v9/*:: as any*/),
        (v10/*:: as any*/),
        (v11/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "concreteType": "ChannelType",
          "kind": "LinkedField",
          "name": "channelType",
          "plural": false,
          "selections": [
            (v6/*:: as any*/)
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
        (v9/*:: as any*/),
        (v10/*:: as any*/),
        (v12/*:: as any*/),
        (v13/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "concreteType": "Service",
          "kind": "LinkedField",
          "name": "sharedFrom",
          "plural": false,
          "selections": (v14/*:: as any*/),
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v16 = [
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "recordings"
  }
],
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedUntil",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "watchPositionMs",
  "storageKey": null
},
v21 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v22 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "previewUrl",
  "storageKey": null
},
v23 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v10/*:: as any*/),
    (v11/*:: as any*/)
  ],
  "storageKey": null
},
v24 = {
  "kind": "Literal",
  "name": "enabled",
  "value": true
},
v25 = {
  "kind": "Variable",
  "name": "to",
  "variableName": "to"
},
v26 = [
  (v24/*:: as any*/),
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "upcoming"
  },
  (v8/*:: as any*/),
  (v25/*:: as any*/)
],
v27 = {
  "alias": null,
  "args": null,
  "concreteType": "ReservationAssignment",
  "kind": "LinkedField",
  "name": "assignment",
  "plural": false,
  "selections": [
    (v17/*:: as any*/)
  ],
  "storageKey": null
},
v28 = {
  "alias": "troubles",
  "args": [
    (v24/*:: as any*/),
    {
      "kind": "Literal",
      "name": "first",
      "value": 100
    },
    (v8/*:: as any*/),
    {
      "kind": "Literal",
      "name": "states",
      "value": [
        "CONFLICT",
        "PARTIAL"
      ]
    },
    (v25/*:: as any*/)
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
          "selections": (v14/*:: as any*/),
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
v29 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v10/*:: as any*/),
    (v11/*:: as any*/),
    (v9/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      (v3/*:: as any*/),
      (v4/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "DashboardQuery",
    "selections": [
      (v5/*:: as any*/),
      (v7/*:: as any*/),
      (v15/*:: as any*/),
      {
        "alias": null,
        "args": (v16/*:: as any*/),
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
                  (v9/*:: as any*/),
                  (v10/*:: as any*/),
                  (v12/*:: as any*/),
                  (v13/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  (v19/*:: as any*/),
                  (v20/*:: as any*/),
                  (v21/*:: as any*/),
                  (v22/*:: as any*/),
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
      {
        "alias": "upcoming",
        "args": (v26/*:: as any*/),
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
                  (v9/*:: as any*/),
                  (v10/*:: as any*/),
                  (v12/*:: as any*/),
                  (v13/*:: as any*/),
                  (v23/*:: as any*/),
                  (v27/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v28/*:: as any*/)
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
      (v2/*:: as any*/),
      (v4/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "DashboardQuery",
    "selections": [
      (v5/*:: as any*/),
      (v7/*:: as any*/),
      (v15/*:: as any*/),
      {
        "alias": null,
        "args": (v16/*:: as any*/),
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
                  (v9/*:: as any*/),
                  (v10/*:: as any*/),
                  (v12/*:: as any*/),
                  (v13/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  (v19/*:: as any*/),
                  (v20/*:: as any*/),
                  (v21/*:: as any*/),
                  (v22/*:: as any*/),
                  (v29/*:: as any*/)
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
        "args": (v26/*:: as any*/),
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
                  (v9/*:: as any*/),
                  (v10/*:: as any*/),
                  (v12/*:: as any*/),
                  (v13/*:: as any*/),
                  (v29/*:: as any*/),
                  (v27/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v28/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "4f538e2b1776b9e5c2745e0303513978",
    "id": null,
    "metadata": {},
    "name": "DashboardQuery",
    "operationKind": "query",
    "text": "query DashboardQuery(\n  $from: Time!\n  $nowEnd: Time!\n  $to: Time!\n  $recordings: Int!\n  $upcoming: Int!\n) {\n  tunerServerHealth {\n    degraded\n    degradedSince\n  }\n  channelTypes {\n    type\n    displayName\n  }\n  guide(from: $from, to: $nowEnd) {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        type\n      }\n    }\n    programs {\n      id\n      name\n      startAt\n      endAt\n      sharedFrom {\n        id\n      }\n    }\n  }\n  recordings(first: $recordings) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        state\n        recordedFrom\n        recordedUntil\n        watchPositionMs\n        thumbnailUrl\n        previewUrl\n        service {\n          name\n          logoUrl\n          id\n        }\n      }\n    }\n  }\n  upcoming: reservations(from: $from, to: $to, enabled: true, first: $upcoming) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        service {\n          name\n          logoUrl\n          id\n        }\n        assignment {\n          state\n        }\n      }\n    }\n  }\n  troubles: reservations(from: $from, to: $to, states: [CONFLICT, PARTIAL], enabled: true, first: 100) {\n    edges {\n      node {\n        id\n      }\n    }\n    pageInfo {\n      hasNextPage\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "b453e05fc8aea04f9f7fd4039f8c5f4f";

export default node;
