/**
 * @generated SignedSource<<1b11c6e3267a76f604f94b31028b0130>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaWatchStatus = "WANNA_WATCH" | "WATCHING" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type DramaLibraryQuery$variables = Record<PropertyKey, never>;
export type DramaLibraryQuery$data = {
  readonly dramaLibrary: {
    readonly enabled: boolean;
    readonly works: ReadonlyArray<{
      readonly id: string;
      readonly lastWatched: {
        readonly episodeNumber: number;
        readonly name: string | null | undefined;
        readonly seasonNumber: number;
      } | null | undefined;
      readonly nextBroadcast: {
        readonly episodeNumber: number | null | undefined;
        readonly last: boolean;
        readonly program: {
          readonly endAt: string;
          readonly id: string;
          readonly name: string;
          readonly reservation: {
            readonly enabled: boolean;
          } | null | undefined;
          readonly service: {
            readonly channelType: {
              readonly type: string;
            };
            readonly logoUrl: string | null | undefined;
            readonly name: string;
          };
          readonly startAt: string;
        };
        readonly rerun: boolean;
        readonly seasonNumber: number | null | undefined;
      } | null | undefined;
      readonly nextEpisode: {
        readonly episodeNumber: number;
        readonly name: string | null | undefined;
        readonly seasonNumber: number;
        readonly url: string;
      } | null | undefined;
      readonly recentBroadcast: {
        readonly last: boolean;
        readonly startAt: string;
      } | null | undefined;
      readonly recordedEpisodes: ReadonlyArray<{
        readonly episodeNumber: number;
        readonly name: string | null | undefined;
        readonly recording: {
          readonly id: string;
        };
        readonly recordingCount: number;
        readonly seasonNumber: number;
        readonly watched: boolean | null | undefined;
      }>;
      readonly series: {
        readonly name: string | null | undefined;
        readonly originalName: string | null | undefined;
        readonly tmdbId: number;
        readonly url: string;
      };
      readonly status: DramaWatchStatus;
    }>;
  };
  readonly viewer: {
    readonly role: Role;
  };
};
export type DramaLibraryQuery = {
  response: DramaLibraryQuery$data;
  variables: DramaLibraryQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "enabled",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "status",
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
  "name": "url",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "DramaSeries",
  "kind": "LinkedField",
  "name": "series",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "tmdbId",
      "storageKey": null
    },
    (v4/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "originalName",
      "storageKey": null
    },
    (v5/*:: as any*/)
  ],
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "seasonNumber",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "episodeNumber",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "concreteType": "DramaLibraryEpisode",
  "kind": "LinkedField",
  "name": "lastWatched",
  "plural": false,
  "selections": [
    (v7/*:: as any*/),
    (v8/*:: as any*/),
    (v4/*:: as any*/)
  ],
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "concreteType": "DramaLibraryEpisode",
  "kind": "LinkedField",
  "name": "nextEpisode",
  "plural": false,
  "selections": [
    (v7/*:: as any*/),
    (v8/*:: as any*/),
    (v4/*:: as any*/),
    (v5/*:: as any*/)
  ],
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "rerun",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "last",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v16 = {
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
v17 = {
  "alias": null,
  "args": null,
  "concreteType": "DramaLibraryPastBroadcast",
  "kind": "LinkedField",
  "name": "recentBroadcast",
  "plural": false,
  "selections": [
    (v13/*:: as any*/),
    (v12/*:: as any*/)
  ],
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "concreteType": "DramaLibraryRecordedEpisode",
  "kind": "LinkedField",
  "name": "recordedEpisodes",
  "plural": true,
  "selections": [
    (v7/*:: as any*/),
    (v8/*:: as any*/),
    (v4/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "recordingCount",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "watched",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "Recording",
      "kind": "LinkedField",
      "name": "recording",
      "plural": false,
      "selections": [
        (v2/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "DramaLibraryQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "DramaLibrary",
        "kind": "LinkedField",
        "name": "dramaLibrary",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "DramaLibraryWork",
            "kind": "LinkedField",
            "name": "works",
            "plural": true,
            "selections": [
              (v2/*:: as any*/),
              (v3/*:: as any*/),
              (v6/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "DramaLibraryBroadcast",
                "kind": "LinkedField",
                "name": "nextBroadcast",
                "plural": false,
                "selections": [
                  (v7/*:: as any*/),
                  (v8/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Program",
                    "kind": "LinkedField",
                    "name": "program",
                    "plural": false,
                    "selections": [
                      (v2/*:: as any*/),
                      (v4/*:: as any*/),
                      (v13/*:: as any*/),
                      (v14/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Reservation",
                        "kind": "LinkedField",
                        "name": "reservation",
                        "plural": false,
                        "selections": [
                          (v1/*:: as any*/)
                        ],
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
                          (v4/*:: as any*/),
                          (v15/*:: as any*/),
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
              (v17/*:: as any*/),
              (v18/*:: as any*/)
            ],
            "storageKey": null
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
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DramaLibraryQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/),
          (v2/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "DramaLibrary",
        "kind": "LinkedField",
        "name": "dramaLibrary",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "DramaLibraryWork",
            "kind": "LinkedField",
            "name": "works",
            "plural": true,
            "selections": [
              (v2/*:: as any*/),
              (v3/*:: as any*/),
              (v6/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "DramaLibraryBroadcast",
                "kind": "LinkedField",
                "name": "nextBroadcast",
                "plural": false,
                "selections": [
                  (v7/*:: as any*/),
                  (v8/*:: as any*/),
                  (v11/*:: as any*/),
                  (v12/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Program",
                    "kind": "LinkedField",
                    "name": "program",
                    "plural": false,
                    "selections": [
                      (v2/*:: as any*/),
                      (v4/*:: as any*/),
                      (v13/*:: as any*/),
                      (v14/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Reservation",
                        "kind": "LinkedField",
                        "name": "reservation",
                        "plural": false,
                        "selections": [
                          (v1/*:: as any*/),
                          (v2/*:: as any*/)
                        ],
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
                          (v4/*:: as any*/),
                          (v15/*:: as any*/),
                          (v16/*:: as any*/),
                          (v2/*:: as any*/)
                        ],
                        "storageKey": null
                      }
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              },
              (v17/*:: as any*/),
              (v18/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "2d6bf4b6bf435270587b943740cb6055",
    "id": null,
    "metadata": {},
    "name": "DramaLibraryQuery",
    "operationKind": "query",
    "text": "query DramaLibraryQuery {\n  viewer {\n    role\n    id\n  }\n  dramaLibrary {\n    enabled\n    works {\n      id\n      status\n      series {\n        tmdbId\n        name\n        originalName\n        url\n      }\n      lastWatched {\n        seasonNumber\n        episodeNumber\n        name\n      }\n      nextEpisode {\n        seasonNumber\n        episodeNumber\n        name\n        url\n      }\n      nextBroadcast {\n        seasonNumber\n        episodeNumber\n        rerun\n        last\n        program {\n          id\n          name\n          startAt\n          endAt\n          reservation {\n            enabled\n            id\n          }\n          service {\n            name\n            logoUrl\n            channelType {\n              type\n            }\n            id\n          }\n        }\n      }\n      recentBroadcast {\n        startAt\n        last\n      }\n      recordedEpisodes {\n        seasonNumber\n        episodeNumber\n        name\n        recordingCount\n        watched\n        recording {\n          id\n        }\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "5bb87d63ef68892dcd231fd527b0e6a2";

export default node;
