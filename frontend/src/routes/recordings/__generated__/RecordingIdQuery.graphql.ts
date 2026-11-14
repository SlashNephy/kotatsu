/**
 * @generated SignedSource<<f8f450dd704ebbbcec36388c487bacd0>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingCMDetectionState = "DETECTED" | "FAILED" | "NOT_DETECTED" | "QUEUED" | "RUNNING" | "%future added value";
export type RecordingFailureReason = "CANCELLED" | "PROGRAM_NOT_FOUND" | "RECORDER_UNAVAILABLE" | "STORAGE_FULL" | "STORAGE_UNAVAILABLE" | "STREAM_INTERRUPTED" | "TUNER_PREEMPTED" | "TUNER_SERVER_UNREACHABLE" | "TUNER_UNAVAILABLE" | "%future added value";
export type RecordingFileKind = "OTHER" | "SEGMENT" | "%future added value";
export type RecordingState = "COMPLETED" | "COMPLETED_WITH_ERRORS" | "FAILED" | "PARTIAL" | "RECORDING" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type StreamKind = "AUDIO" | "OTHER" | "PSI" | "UNKNOWN" | "VIDEO" | "%future added value";
export type VideoFormat = "HD" | "SHV" | "UHD" | "%future added value";
export type RecordingIdQuery$variables = {
  id: string;
};
export type RecordingIdQuery$data = {
  readonly node: {
    readonly __typename: "Recording";
    readonly animeEpisodes: ReadonlyArray<{
      readonly countEnd: number | null | undefined;
      readonly countStart: number | null | undefined;
      readonly first: boolean;
      readonly last: boolean;
      readonly rerun: boolean;
      readonly subtitle: string | null | undefined;
      readonly syobocalTid: number | null | undefined;
      readonly syobocalUrl: string | null | undefined;
      readonly works: ReadonlyArray<{
        readonly anilistUrl: string | null | undefined;
        readonly annictUrl: string | null | undefined;
        readonly malUrl: string | null | undefined;
      }>;
    }>;
    readonly broadcastEndedAt: string | null | undefined;
    readonly broadcastStartedAt: string | null | undefined;
    readonly cmDetection: {
      readonly detectedAt: string | null | undefined;
      readonly note: string | null | undefined;
      readonly progress: {
        readonly ratio: number;
        readonly remainingSeconds: number | null | undefined;
      } | null | undefined;
      readonly state: RecordingCMDetectionState;
      readonly videoMissingSeconds: number;
    };
    readonly cmSections: ReadonlyArray<{
      readonly from: string;
    }>;
    readonly description: string;
    readonly drops: number;
    readonly endAt: string;
    readonly errors: number;
    readonly failureDetail: string | null | undefined;
    readonly failureReason: RecordingFailureReason | null | undefined;
    readonly files: ReadonlyArray<{
      readonly kind: RecordingFileKind;
      readonly path: string;
      readonly size: number;
      readonly transcodeFrameRate: number | null | undefined;
      readonly tuner: {
        readonly index: number;
        readonly name: string;
      } | null | undefined;
      readonly url: string | null | undefined;
      readonly video: {
        readonly codec: string;
        readonly frameRate: number | null | undefined;
        readonly height: number;
        readonly interlaced: boolean;
        readonly width: number;
      } | null | undefined;
    }>;
    readonly gaps: ReadonlyArray<{
      readonly from: string;
      readonly to: string;
    }>;
    readonly genres: ReadonlyArray<{
      readonly lv1: number;
      readonly lv2: number;
    }>;
    readonly id: string;
    readonly jikkyoCommentsUrl: string;
    readonly name: string;
    readonly notBroadcast: boolean;
    readonly programLost: boolean;
    readonly protected: boolean;
    readonly recordedFrom: string;
    readonly recordedUntil: string | null | undefined;
    readonly rule: {
      readonly id: string;
    } | null | undefined;
    readonly ruleName: string | null | undefined;
    readonly scrambleWarning: boolean;
    readonly scrambled: number;
    readonly scrambledRatio: number | null | undefined;
    readonly service: {
      readonly jikkyo: {
        readonly id: string;
      } | null | undefined;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
    readonly startAt: string;
    readonly state: RecordingState;
    readonly streamQuality: ReadonlyArray<{
      readonly componentTag: number | null | undefined;
      readonly drops: number;
      readonly errors: number;
      readonly kind: StreamKind;
      readonly packets: number;
      readonly pid: number;
      readonly scrambled: number;
      readonly streamType: number | null | undefined;
    }>;
    readonly streamUrl: string;
    readonly tags: ReadonlyArray<{
      readonly id: string;
      readonly name: string;
    }>;
    readonly thumbnailUrl: string | null | undefined;
    readonly tvtplayChapterUrl: string;
    readonly videoFormat: VideoFormat;
    readonly workTitle: string;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly systemSettings: {
    readonly externalPlayerUrlsEnabled: boolean;
    readonly recordingDownloadsEnabled: boolean;
  };
  readonly tags: ReadonlyArray<{
    readonly name: string;
  }>;
  readonly viewer: {
    readonly role: Role;
  };
};
export type RecordingIdQuery = {
  response: RecordingIdQuery$data;
  variables: RecordingIdQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
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
  "concreteType": "SystemSettings",
  "kind": "LinkedField",
  "name": "systemSettings",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "externalPlayerUrlsEnabled",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "recordingDownloadsEnabled",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v4 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "workTitle",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "description",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "state",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "failureReason",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "failureDetail",
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "protected",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "drops",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "errors",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "scrambled",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "scrambleWarning",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "scrambledRatio",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v21 = {
  "alias": null,
  "args": null,
  "concreteType": "StreamQuality",
  "kind": "LinkedField",
  "name": "streamQuality",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "pid",
      "storageKey": null
    },
    (v20/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "packets",
      "storageKey": null
    },
    (v15/*:: as any*/),
    (v16/*:: as any*/),
    (v17/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "streamType",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "componentTag",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v22 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "from",
  "storageKey": null
},
v23 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingGap",
  "kind": "LinkedField",
  "name": "gaps",
  "plural": true,
  "selections": [
    (v22/*:: as any*/),
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
v24 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingCMSection",
  "kind": "LinkedField",
  "name": "cmSections",
  "plural": true,
  "selections": [
    (v22/*:: as any*/)
  ],
  "storageKey": null
},
v25 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingCMDetection",
  "kind": "LinkedField",
  "name": "cmDetection",
  "plural": false,
  "selections": [
    (v11/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "detectedAt",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "note",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "videoMissingSeconds",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "JobProgress",
      "kind": "LinkedField",
      "name": "progress",
      "plural": false,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "ratio",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "remainingSeconds",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v26 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "programLost",
  "storageKey": null
},
v27 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "broadcastStartedAt",
  "storageKey": null
},
v28 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "broadcastEndedAt",
  "storageKey": null
},
v29 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "notBroadcast",
  "storageKey": null
},
v30 = {
  "alias": null,
  "args": null,
  "concreteType": "Genre",
  "kind": "LinkedField",
  "name": "genres",
  "plural": true,
  "selections": [
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
  "storageKey": null
},
v31 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeEpisode",
  "kind": "LinkedField",
  "name": "animeEpisodes",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "syobocalTid",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "syobocalUrl",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "countStart",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "countEnd",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "subtitle",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "first",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "last",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "rerun",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "AnimeWork",
      "kind": "LinkedField",
      "name": "works",
      "plural": true,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "annictUrl",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "anilistUrl",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "malUrl",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v32 = [
  (v6/*:: as any*/)
],
v33 = {
  "alias": null,
  "args": null,
  "concreteType": "Rule",
  "kind": "LinkedField",
  "name": "rule",
  "plural": false,
  "selections": (v32/*:: as any*/),
  "storageKey": null
},
v34 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "ruleName",
  "storageKey": null
},
v35 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "thumbnailUrl",
  "storageKey": null
},
v36 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "streamUrl",
  "storageKey": null
},
v37 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "videoFormat",
  "storageKey": null
},
v38 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "jikkyoCommentsUrl",
  "storageKey": null
},
v39 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "tvtplayChapterUrl",
  "storageKey": null
},
v40 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v41 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "recordedUntil",
  "storageKey": null
},
v42 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v43 = {
  "alias": null,
  "args": null,
  "concreteType": "JikkyoChannel",
  "kind": "LinkedField",
  "name": "jikkyo",
  "plural": false,
  "selections": (v32/*:: as any*/),
  "storageKey": null
},
v44 = {
  "alias": null,
  "args": null,
  "concreteType": "Tag",
  "kind": "LinkedField",
  "name": "tags",
  "plural": true,
  "selections": [
    (v6/*:: as any*/),
    (v3/*:: as any*/)
  ],
  "storageKey": null
},
v45 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingFile",
  "kind": "LinkedField",
  "name": "files",
  "plural": true,
  "selections": [
    (v20/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "path",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "size",
      "storageKey": null
    },
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
      "concreteType": "SegmentVideoFormat",
      "kind": "LinkedField",
      "name": "video",
      "plural": false,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "codec",
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
          "name": "interlaced",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "frameRate",
          "storageKey": null
        }
      ],
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "RecordingFileTuner",
      "kind": "LinkedField",
      "name": "tuner",
      "plural": false,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "index",
          "storageKey": null
        },
        (v3/*:: as any*/)
      ],
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "transcodeFrameRate",
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
    "name": "RecordingIdQuery",
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
      (v2/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "Tag",
        "kind": "LinkedField",
        "name": "tags",
        "plural": true,
        "selections": [
          (v3/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v4/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v5/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v6/*:: as any*/),
              (v3/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              (v15/*:: as any*/),
              (v16/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/),
              (v21/*:: as any*/),
              (v23/*:: as any*/),
              (v24/*:: as any*/),
              (v25/*:: as any*/),
              (v26/*:: as any*/),
              (v27/*:: as any*/),
              (v28/*:: as any*/),
              (v29/*:: as any*/),
              (v30/*:: as any*/),
              (v31/*:: as any*/),
              (v33/*:: as any*/),
              (v34/*:: as any*/),
              (v35/*:: as any*/),
              (v36/*:: as any*/),
              (v37/*:: as any*/),
              (v38/*:: as any*/),
              (v39/*:: as any*/),
              (v40/*:: as any*/),
              (v41/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v42/*:: as any*/),
                  (v43/*:: as any*/)
                ],
                "storageKey": null
              },
              (v44/*:: as any*/),
              (v45/*:: as any*/)
            ],
            "type": "Recording",
            "abstractKey": null
          }
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
    "name": "RecordingIdQuery",
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
          (v6/*:: as any*/)
        ],
        "storageKey": null
      },
      (v2/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "Tag",
        "kind": "LinkedField",
        "name": "tags",
        "plural": true,
        "selections": [
          (v3/*:: as any*/),
          (v6/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v4/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v3/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              (v14/*:: as any*/),
              (v15/*:: as any*/),
              (v16/*:: as any*/),
              (v17/*:: as any*/),
              (v18/*:: as any*/),
              (v19/*:: as any*/),
              (v21/*:: as any*/),
              (v23/*:: as any*/),
              (v24/*:: as any*/),
              (v25/*:: as any*/),
              (v26/*:: as any*/),
              (v27/*:: as any*/),
              (v28/*:: as any*/),
              (v29/*:: as any*/),
              (v30/*:: as any*/),
              (v31/*:: as any*/),
              (v33/*:: as any*/),
              (v34/*:: as any*/),
              (v35/*:: as any*/),
              (v36/*:: as any*/),
              (v37/*:: as any*/),
              (v38/*:: as any*/),
              (v39/*:: as any*/),
              (v40/*:: as any*/),
              (v41/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v3/*:: as any*/),
                  (v42/*:: as any*/),
                  (v43/*:: as any*/),
                  (v6/*:: as any*/)
                ],
                "storageKey": null
              },
              (v44/*:: as any*/),
              (v45/*:: as any*/)
            ],
            "type": "Recording",
            "abstractKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "3c69047fdae77531b9971ee4edb3a199",
    "id": null,
    "metadata": {},
    "name": "RecordingIdQuery",
    "operationKind": "query",
    "text": "query RecordingIdQuery(\n  $id: ID!\n) {\n  viewer {\n    role\n    id\n  }\n  systemSettings {\n    externalPlayerUrlsEnabled\n    recordingDownloadsEnabled\n  }\n  tags {\n    name\n    id\n  }\n  node(id: $id) {\n    __typename\n    ... on Recording {\n      id\n      name\n      workTitle\n      description\n      startAt\n      endAt\n      state\n      failureReason\n      failureDetail\n      protected\n      drops\n      errors\n      scrambled\n      scrambleWarning\n      scrambledRatio\n      streamQuality {\n        pid\n        kind\n        packets\n        drops\n        errors\n        scrambled\n        streamType\n        componentTag\n      }\n      gaps {\n        from\n        to\n      }\n      cmSections {\n        from\n      }\n      cmDetection {\n        state\n        detectedAt\n        note\n        videoMissingSeconds\n        progress {\n          ratio\n          remainingSeconds\n        }\n      }\n      programLost\n      broadcastStartedAt\n      broadcastEndedAt\n      notBroadcast\n      genres {\n        lv1\n        lv2\n      }\n      animeEpisodes {\n        syobocalTid\n        syobocalUrl\n        countStart\n        countEnd\n        subtitle\n        first\n        last\n        rerun\n        works {\n          annictUrl\n          anilistUrl\n          malUrl\n        }\n      }\n      rule {\n        id\n      }\n      ruleName\n      thumbnailUrl\n      streamUrl\n      videoFormat\n      jikkyoCommentsUrl\n      tvtplayChapterUrl\n      recordedFrom\n      recordedUntil\n      service {\n        name\n        logoUrl\n        jikkyo {\n          id\n        }\n        id\n      }\n      tags {\n        id\n        name\n      }\n      files {\n        kind\n        path\n        size\n        url\n        video {\n          codec\n          width\n          height\n          interlaced\n          frameRate\n        }\n        tuner {\n          index\n          name\n        }\n        transcodeFrameRate\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "f3a4a2d3a590cac722ae8f5de4bf4295";

export default node;
