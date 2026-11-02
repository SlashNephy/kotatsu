/**
 * @generated SignedSource<<2ae0ff1f0dc7fadf1a7c6476046b118f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type AnimeSeason = "AUTUMN" | "SPRING" | "SUMMER" | "WINTER" | "%future added value";
export type AnimeWishlistStatus = "WANNA_WATCH" | "WATCHING" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type AnimeLibraryQuery$variables = Record<PropertyKey, never>;
export type AnimeLibraryQuery$data = {
  readonly animeLibrary: {
    readonly enabled: boolean;
    readonly imports: ReadonlyArray<{
      readonly destination: AnimeRecordDestination;
      readonly entryCount: number;
      readonly error: string | null | undefined;
      readonly importedAt: string | null | undefined;
    }>;
    readonly works: ReadonlyArray<{
      readonly broadcasts: ReadonlyArray<{
        readonly countEnd: number | null | undefined;
        readonly countStart: number | null | undefined;
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
      }>;
      readonly episodesCount: number | null | undefined;
      readonly id: string;
      readonly nextEpisode: {
        readonly number: number | null | undefined;
        readonly numberText: string | null | undefined;
        readonly title: string | null | undefined;
      } | null | undefined;
      readonly progress: number | null | undefined;
      readonly recentBroadcasts: ReadonlyArray<{
        readonly last: boolean;
        readonly rerun: boolean;
        readonly startAt: string;
      }>;
      readonly recordedEpisodes: ReadonlyArray<{
        readonly countEnd: number;
        readonly countStart: number;
        readonly recording: {
          readonly id: string;
        };
        readonly recordingCount: number;
        readonly subtitle: string | null | undefined;
        readonly watched: boolean | null | undefined;
      }>;
      readonly season: AnimeSeason | null | undefined;
      readonly seasonYear: number | null | undefined;
      readonly sources: ReadonlyArray<{
        readonly destination: AnimeRecordDestination;
        readonly status: AnimeWishlistStatus;
        readonly url: string;
        readonly workId: number;
      }>;
      readonly status: AnimeWishlistStatus;
      readonly tid: number | null | undefined;
      readonly title: string;
    }>;
  };
  readonly animeWishlist: {
    readonly works: ReadonlyArray<{
      readonly covered: boolean;
      readonly tid: number;
    }>;
  };
  readonly viewer: {
    readonly role: Role;
  };
};
export type AnimeLibraryQuery = {
  response: AnimeLibraryQuery$data;
  variables: AnimeLibraryQuery$variables;
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
  "name": "destination",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeWishlistImport",
  "kind": "LinkedField",
  "name": "imports",
  "plural": true,
  "selections": [
    (v2/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "importedAt",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "error",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "entryCount",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "tid",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "title",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "status",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "progress",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "episodesCount",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "seasonYear",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "season",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeLibrarySource",
  "kind": "LinkedField",
  "name": "sources",
  "plural": true,
  "selections": [
    (v2/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "workId",
      "storageKey": null
    },
    (v7/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "url",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeLibraryEpisode",
  "kind": "LinkedField",
  "name": "nextEpisode",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "number",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "numberText",
      "storageKey": null
    },
    (v6/*:: as any*/)
  ],
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "countStart",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "countEnd",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "rerun",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "last",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
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
  "concreteType": "AnimeLibraryPastBroadcast",
  "kind": "LinkedField",
  "name": "recentBroadcasts",
  "plural": true,
  "selections": [
    (v19/*:: as any*/),
    (v16/*:: as any*/),
    (v17/*:: as any*/)
  ],
  "storageKey": null
},
v24 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeLibraryRecordedEpisode",
  "kind": "LinkedField",
  "name": "recordedEpisodes",
  "plural": true,
  "selections": [
    (v14/*:: as any*/),
    (v15/*:: as any*/),
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
        (v4/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v25 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeWishlist",
  "kind": "LinkedField",
  "name": "animeWishlist",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "concreteType": "AnimeWishlistWork",
      "kind": "LinkedField",
      "name": "works",
      "plural": true,
      "selections": [
        (v5/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "covered",
          "storageKey": null
        }
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
    "name": "AnimeLibraryQuery",
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
        "concreteType": "AnimeLibrary",
        "kind": "LinkedField",
        "name": "animeLibrary",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "AnimeLibraryWork",
            "kind": "LinkedField",
            "name": "works",
            "plural": true,
            "selections": [
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "AnimeLibraryBroadcast",
                "kind": "LinkedField",
                "name": "broadcasts",
                "plural": true,
                "selections": [
                  (v14/*:: as any*/),
                  (v15/*:: as any*/),
                  (v16/*:: as any*/),
                  (v17/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Program",
                    "kind": "LinkedField",
                    "name": "program",
                    "plural": false,
                    "selections": [
                      (v4/*:: as any*/),
                      (v18/*:: as any*/),
                      (v19/*:: as any*/),
                      (v20/*:: as any*/),
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
                          (v18/*:: as any*/),
                          (v21/*:: as any*/),
                          (v22/*:: as any*/)
                        ],
                        "storageKey": null
                      }
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              },
              (v23/*:: as any*/),
              (v24/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v25/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "AnimeLibraryQuery",
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
          (v4/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "AnimeLibrary",
        "kind": "LinkedField",
        "name": "animeLibrary",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "AnimeLibraryWork",
            "kind": "LinkedField",
            "name": "works",
            "plural": true,
            "selections": [
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v13/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "AnimeLibraryBroadcast",
                "kind": "LinkedField",
                "name": "broadcasts",
                "plural": true,
                "selections": [
                  (v14/*:: as any*/),
                  (v15/*:: as any*/),
                  (v16/*:: as any*/),
                  (v17/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Program",
                    "kind": "LinkedField",
                    "name": "program",
                    "plural": false,
                    "selections": [
                      (v4/*:: as any*/),
                      (v18/*:: as any*/),
                      (v19/*:: as any*/),
                      (v20/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Reservation",
                        "kind": "LinkedField",
                        "name": "reservation",
                        "plural": false,
                        "selections": [
                          (v1/*:: as any*/),
                          (v4/*:: as any*/)
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
                          (v18/*:: as any*/),
                          (v21/*:: as any*/),
                          (v22/*:: as any*/),
                          (v4/*:: as any*/)
                        ],
                        "storageKey": null
                      }
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              },
              (v23/*:: as any*/),
              (v24/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v25/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "3a8224a1ccb6d056cefb6f71639ffb1a",
    "id": null,
    "metadata": {},
    "name": "AnimeLibraryQuery",
    "operationKind": "query",
    "text": "query AnimeLibraryQuery {\n  viewer {\n    role\n    id\n  }\n  animeLibrary {\n    enabled\n    imports {\n      destination\n      importedAt\n      error\n      entryCount\n    }\n    works {\n      id\n      tid\n      title\n      status\n      progress\n      episodesCount\n      seasonYear\n      season\n      sources {\n        destination\n        workId\n        status\n        url\n      }\n      nextEpisode {\n        number\n        numberText\n        title\n      }\n      broadcasts {\n        countStart\n        countEnd\n        rerun\n        last\n        program {\n          id\n          name\n          startAt\n          endAt\n          reservation {\n            enabled\n            id\n          }\n          service {\n            name\n            logoUrl\n            channelType {\n              type\n            }\n            id\n          }\n        }\n      }\n      recentBroadcasts {\n        startAt\n        rerun\n        last\n      }\n      recordedEpisodes {\n        countStart\n        countEnd\n        subtitle\n        recordingCount\n        watched\n        recording {\n          id\n        }\n      }\n    }\n  }\n  animeWishlist {\n    works {\n      tid\n      covered\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "47356bd0073e7ae699edf747dede091a";

export default node;
