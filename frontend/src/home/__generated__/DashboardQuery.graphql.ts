/**
 * @generated SignedSource<<a66df003f38a60fb8d6ff1041409ec05>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AssignmentState = "CONFLICT" | "OK" | "PARTIAL" | "PENDING" | "%future added value";
export type RecordingState = "COMPLETED" | "COMPLETED_WITH_ERRORS" | "FAILED" | "PARTIAL" | "RECORDING" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
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
  readonly continueWatching: ReadonlyArray<{
    readonly drops: number;
    readonly endAt: string;
    readonly errors: number;
    readonly gaps: ReadonlyArray<{
      readonly from: string;
      readonly to: string;
    }>;
    readonly id: string;
    readonly name: string;
    readonly previewUrl: string | null | undefined;
    readonly recordedFrom: string;
    readonly recordedUntil: string | null | undefined;
    readonly scrambled: number;
    readonly service: {
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
    readonly startAt: string;
    readonly state: RecordingState;
    readonly thumbnailUrl: string | null | undefined;
    readonly watchFrame: {
      readonly height: number;
      readonly sheetHeight: number;
      readonly sheetWidth: number;
      readonly url: string;
      readonly width: number;
      readonly x: number;
      readonly y: number;
    } | null | undefined;
    readonly watchPositionMs: number | null | undefined;
  }>;
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly endAt: string;
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
      readonly jikkyo: {
        readonly force: number | null | undefined;
        readonly id: string;
      } | null | undefined;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
  }>;
  readonly nextEpisodes: ReadonlyArray<{
    readonly program: {
      readonly id: string;
      readonly name: string;
      readonly ruleKeyword: string;
      readonly service: {
        readonly id: string;
        readonly logoUrl: string | null | undefined;
        readonly name: string;
      };
      readonly startAt: string;
    };
    readonly watched: {
      readonly id: string;
    };
  }>;
  readonly pinnedServices: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly recordings: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly drops: number;
        readonly endAt: string;
        readonly errors: number;
        readonly gaps: ReadonlyArray<{
          readonly from: string;
          readonly to: string;
        }>;
        readonly id: string;
        readonly name: string;
        readonly previewUrl: string | null | undefined;
        readonly recordedFrom: string;
        readonly recordedUntil: string | null | undefined;
        readonly scrambled: number;
        readonly service: {
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
        readonly startAt: string;
        readonly state: RecordingState;
        readonly thumbnailUrl: string | null | undefined;
        readonly watchFrame: {
          readonly height: number;
          readonly sheetHeight: number;
          readonly sheetWidth: number;
          readonly url: string;
          readonly width: number;
          readonly x: number;
          readonly y: number;
        } | null | undefined;
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
    readonly broadcastClockSkewMs: number | null | undefined;
    readonly broadcastClockSkewed: boolean;
    readonly clockSkewMs: number | null | undefined;
    readonly clockSkewed: boolean;
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
          readonly channelType: {
            readonly type: string;
          };
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
        readonly startAt: string;
      };
    }>;
  };
  readonly viewer: {
    readonly role: Role;
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
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "clockSkewed",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "clockSkewMs",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "broadcastClockSkewed",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "broadcastClockSkewMs",
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
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v9 = [
  (v8/*:: as any*/)
],
v10 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "pinnedServices",
  "plural": true,
  "selections": (v9/*:: as any*/),
  "storageKey": null
},
v11 = {
  "kind": "Variable",
  "name": "from",
  "variableName": "from"
},
v12 = [
  (v11/*:: as any*/),
  {
    "kind": "Variable",
    "name": "to",
    "variableName": "nowEnd"
  }
],
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v15 = {
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
},
v16 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v8/*:: as any*/),
    (v13/*:: as any*/),
    (v14/*:: as any*/),
    (v15/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "concreteType": "JikkyoChannel",
      "kind": "LinkedField",
      "name": "jikkyo",
      "plural": false,
      "selections": [
        (v8/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "force",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
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
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "sharedFrom",
  "plural": false,
  "selections": (v9/*:: as any*/),
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v21 = {
  "alias": null,
  "args": null,
  "concreteType": "SeriesNextEpisode",
  "kind": "LinkedField",
  "name": "nextEpisodes",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "concreteType": "Recording",
      "kind": "LinkedField",
      "name": "watched",
      "plural": false,
      "selections": (v9/*:: as any*/),
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "Program",
      "kind": "LinkedField",
      "name": "program",
      "plural": false,
      "selections": [
        (v8/*:: as any*/),
        (v13/*:: as any*/),
        (v17/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "ruleKeyword",
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
            (v8/*:: as any*/),
            (v13/*:: as any*/),
            (v14/*:: as any*/)
          ],
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v22 = [
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "recordings"
  }
],
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
  "name": "drops",
  "storageKey": null
},
v25 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "errors",
  "storageKey": null
},
v26 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "scrambled",
  "storageKey": null
},
v27 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v28 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedUntil",
  "storageKey": null
},
v29 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingGap",
  "kind": "LinkedField",
  "name": "gaps",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "from",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "to",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v30 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "watchPositionMs",
  "storageKey": null
},
v31 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v32 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "previewUrl",
  "storageKey": null
},
v33 = {
  "alias": null,
  "args": null,
  "concreteType": "PreviewFrame",
  "kind": "LinkedField",
  "name": "watchFrame",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "url",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "x",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "y",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "width",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "height",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "sheetWidth",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "sheetHeight",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v34 = [
  (v8/*:: as any*/),
  (v13/*:: as any*/),
  (v17/*:: as any*/),
  (v18/*:: as any*/),
  (v23/*:: as any*/),
  (v24/*:: as any*/),
  (v25/*:: as any*/),
  (v26/*:: as any*/),
  (v27/*:: as any*/),
  (v28/*:: as any*/),
  (v29/*:: as any*/),
  (v30/*:: as any*/),
  (v31/*:: as any*/),
  (v32/*:: as any*/),
  (v33/*:: as any*/),
  {
    "alias": null,
    "args": null,
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "service",
    "plural": false,
    "selections": [
      (v13/*:: as any*/),
      (v14/*:: as any*/)
    ],
    "storageKey": null
  }
],
v35 = {
  "kind": "Literal",
  "name": "enabled",
  "value": true
},
v36 = {
  "kind": "Variable",
  "name": "to",
  "variableName": "to"
},
v37 = [
  (v35/*:: as any*/),
  {
    "kind": "Variable",
    "name": "first",
    "variableName": "upcoming"
  },
  (v11/*:: as any*/),
  (v36/*:: as any*/)
],
v38 = {
  "alias": null,
  "args": null,
  "concreteType": "ReservationAssignment",
  "kind": "LinkedField",
  "name": "assignment",
  "plural": false,
  "selections": [
    (v23/*:: as any*/)
  ],
  "storageKey": null
},
v39 = {
  "alias": "troubles",
  "args": [
    (v35/*:: as any*/),
    {
      "kind": "Literal",
      "name": "first",
      "value": 100
    },
    (v11/*:: as any*/),
    {
      "kind": "Literal",
      "name": "states",
      "value": [
        "CONFLICT",
        "PARTIAL"
      ]
    },
    (v36/*:: as any*/)
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
          "selections": (v9/*:: as any*/),
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
v40 = [
  (v8/*:: as any*/),
  (v13/*:: as any*/),
  (v17/*:: as any*/),
  (v18/*:: as any*/),
  (v23/*:: as any*/),
  (v24/*:: as any*/),
  (v25/*:: as any*/),
  (v26/*:: as any*/),
  (v27/*:: as any*/),
  (v28/*:: as any*/),
  (v29/*:: as any*/),
  (v30/*:: as any*/),
  (v31/*:: as any*/),
  (v32/*:: as any*/),
  (v33/*:: as any*/),
  {
    "alias": null,
    "args": null,
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "service",
    "plural": false,
    "selections": [
      (v13/*:: as any*/),
      (v14/*:: as any*/),
      (v8/*:: as any*/)
    ],
    "storageKey": null
  }
];
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
      (v10/*:: as any*/),
      {
        "alias": null,
        "args": (v12/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
          (v16/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "programs",
            "plural": true,
            "selections": [
              (v13/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v20/*:: as any*/)
        ],
        "storageKey": null
      },
      (v21/*:: as any*/),
      {
        "alias": null,
        "args": (v22/*:: as any*/),
        "concreteType": "Recording",
        "kind": "LinkedField",
        "name": "continueWatching",
        "plural": true,
        "selections": (v34/*:: as any*/),
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v22/*:: as any*/),
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
                "selections": (v34/*:: as any*/),
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
        "args": (v37/*:: as any*/),
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
                  (v13/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v13/*:: as any*/),
                      (v14/*:: as any*/),
                      (v15/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v38/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v39/*:: as any*/)
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
      (v10/*:: as any*/),
      {
        "alias": null,
        "args": (v12/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
          (v16/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "programs",
            "plural": true,
            "selections": [
              (v13/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/),
              (v8/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v20/*:: as any*/),
          (v8/*:: as any*/)
        ],
        "storageKey": null
      },
      (v21/*:: as any*/),
      {
        "alias": null,
        "args": (v22/*:: as any*/),
        "concreteType": "Recording",
        "kind": "LinkedField",
        "name": "continueWatching",
        "plural": true,
        "selections": (v40/*:: as any*/),
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v22/*:: as any*/),
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
                "selections": (v40/*:: as any*/),
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
        "args": (v37/*:: as any*/),
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
                  (v13/*:: as any*/),
                  (v17/*:: as any*/),
                  (v18/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v13/*:: as any*/),
                      (v14/*:: as any*/),
                      (v15/*:: as any*/),
                      (v8/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v38/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v39/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "28abf10307d8dac0eeede3520dd2ea10",
    "id": null,
    "metadata": {},
    "name": "DashboardQuery",
    "operationKind": "query",
    "text": "query DashboardQuery(\n  $from: Time!\n  $nowEnd: Time!\n  $to: Time!\n  $recordings: Int!\n  $upcoming: Int!\n) {\n  tunerServerHealth {\n    degraded\n    degradedSince\n    clockSkewed\n    clockSkewMs\n    broadcastClockSkewed\n    broadcastClockSkewMs\n  }\n  channelTypes {\n    type\n    displayName\n  }\n  pinnedServices {\n    id\n  }\n  guide(from: $from, to: $nowEnd) {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        type\n      }\n      jikkyo {\n        id\n        force\n      }\n    }\n    programs {\n      name\n      startAt\n      endAt\n      sharedFrom {\n        id\n      }\n      id\n    }\n  }\n  viewer {\n    role\n    id\n  }\n  nextEpisodes {\n    watched {\n      id\n    }\n    program {\n      id\n      name\n      startAt\n      ruleKeyword\n      service {\n        id\n        name\n        logoUrl\n      }\n    }\n  }\n  continueWatching(first: $recordings) {\n    id\n    name\n    startAt\n    endAt\n    state\n    drops\n    errors\n    scrambled\n    recordedFrom\n    recordedUntil\n    gaps {\n      from\n      to\n    }\n    watchPositionMs\n    thumbnailUrl\n    previewUrl\n    watchFrame {\n      url\n      x\n      y\n      width\n      height\n      sheetWidth\n      sheetHeight\n    }\n    service {\n      name\n      logoUrl\n      id\n    }\n  }\n  recordings(first: $recordings) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        state\n        drops\n        errors\n        scrambled\n        recordedFrom\n        recordedUntil\n        gaps {\n          from\n          to\n        }\n        watchPositionMs\n        thumbnailUrl\n        previewUrl\n        watchFrame {\n          url\n          x\n          y\n          width\n          height\n          sheetWidth\n          sheetHeight\n        }\n        service {\n          name\n          logoUrl\n          id\n        }\n      }\n    }\n  }\n  upcoming: reservations(from: $from, to: $to, enabled: true, first: $upcoming) {\n    edges {\n      node {\n        id\n        name\n        startAt\n        endAt\n        service {\n          name\n          logoUrl\n          channelType {\n            type\n          }\n          id\n        }\n        assignment {\n          state\n        }\n      }\n    }\n  }\n  troubles: reservations(from: $from, to: $to, states: [CONFLICT, PARTIAL], enabled: true, first: 100) {\n    edges {\n      node {\n        id\n      }\n    }\n    pageInfo {\n      hasNextPage\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "bb35887471e508860d2f02ef1d14a9e9";

export default node;
