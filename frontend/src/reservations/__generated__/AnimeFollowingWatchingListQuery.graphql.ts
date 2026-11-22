/**
 * @generated SignedSource<<c0188a64f7c9c775078b85b8e46a8eb6>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type AnimeFollowingWatchingListQuery$variables = Record<PropertyKey, never>;
export type AnimeFollowingWatchingListQuery$data = {
  readonly animeFollowingWatching: {
    readonly enabled: boolean;
    readonly works: ReadonlyArray<{
      readonly broadcasts: ReadonlyArray<{
        readonly preferred: boolean;
        readonly program: {
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
      readonly covered: boolean;
      readonly reservedBroadcasts: number;
      readonly tid: number;
      readonly title: string;
      readonly watchers: ReadonlyArray<{
        readonly avatarUrl: string | null | undefined;
        readonly destination: AnimeRecordDestination;
        readonly name: string;
        readonly profileUrl: string;
        readonly username: string;
      }>;
    }>;
  };
  readonly animeRecordSettings: {
    readonly destinations: ReadonlyArray<{
      readonly connection: {
        readonly connectedAt: string;
      } | null | undefined;
    }>;
  };
  readonly viewer: {
    readonly role: Role;
  };
};
export type AnimeFollowingWatchingListQuery = {
  response: AnimeFollowingWatchingListQuery$data;
  variables: AnimeFollowingWatchingListQuery$variables;
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
  "concreteType": "AnimeRecordSettings",
  "kind": "LinkedField",
  "name": "animeRecordSettings",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "concreteType": "AnimeRecordDestinationStatus",
      "kind": "LinkedField",
      "name": "destinations",
      "plural": true,
      "selections": [
        {
          "alias": null,
          "args": null,
          "concreteType": "AnimeRecordConnection",
          "kind": "LinkedField",
          "name": "connection",
          "plural": false,
          "selections": [
            {
              "alias": null,
              "args": null,
              "kind": "ScalarField",
              "name": "connectedAt",
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
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "enabled",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "tid",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "title",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "reservedBroadcasts",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "covered",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeFollowingWatcher",
  "kind": "LinkedField",
  "name": "watchers",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "destination",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "username",
      "storageKey": null
    },
    (v7/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "avatarUrl",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "profileUrl",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "preferred",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
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
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v14 = {
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
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "AnimeFollowingWatchingListQuery",
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
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "AnimeFollowingWatching",
        "kind": "LinkedField",
        "name": "animeFollowingWatching",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "AnimeFollowingWatchingWork",
            "kind": "LinkedField",
            "name": "works",
            "plural": true,
            "selections": [
              (v3/*:: as any*/),
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v8/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "AnimeWishlistBroadcast",
                "kind": "LinkedField",
                "name": "broadcasts",
                "plural": true,
                "selections": [
                  (v9/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Program",
                    "kind": "LinkedField",
                    "name": "program",
                    "plural": false,
                    "selections": [
                      (v10/*:: as any*/),
                      (v7/*:: as any*/),
                      (v11/*:: as any*/),
                      (v12/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Service",
                        "kind": "LinkedField",
                        "name": "service",
                        "plural": false,
                        "selections": [
                          (v7/*:: as any*/),
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
              }
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
    "name": "AnimeFollowingWatchingListQuery",
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
          (v10/*:: as any*/)
        ],
        "storageKey": null
      },
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "AnimeFollowingWatching",
        "kind": "LinkedField",
        "name": "animeFollowingWatching",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "AnimeFollowingWatchingWork",
            "kind": "LinkedField",
            "name": "works",
            "plural": true,
            "selections": [
              (v3/*:: as any*/),
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v8/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "AnimeWishlistBroadcast",
                "kind": "LinkedField",
                "name": "broadcasts",
                "plural": true,
                "selections": [
                  (v9/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Program",
                    "kind": "LinkedField",
                    "name": "program",
                    "plural": false,
                    "selections": [
                      (v10/*:: as any*/),
                      (v7/*:: as any*/),
                      (v11/*:: as any*/),
                      (v12/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Service",
                        "kind": "LinkedField",
                        "name": "service",
                        "plural": false,
                        "selections": [
                          (v7/*:: as any*/),
                          (v13/*:: as any*/),
                          (v14/*:: as any*/),
                          (v10/*:: as any*/)
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
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "dd1b717f7f26b356d3925ee6de2e1b58",
    "id": null,
    "metadata": {},
    "name": "AnimeFollowingWatchingListQuery",
    "operationKind": "query",
    "text": "query AnimeFollowingWatchingListQuery {\n  viewer {\n    role\n    id\n  }\n  animeRecordSettings {\n    destinations {\n      connection {\n        connectedAt\n      }\n    }\n  }\n  animeFollowingWatching {\n    enabled\n    works {\n      tid\n      title\n      reservedBroadcasts\n      covered\n      watchers {\n        destination\n        username\n        name\n        avatarUrl\n        profileUrl\n      }\n      broadcasts {\n        preferred\n        program {\n          id\n          name\n          startAt\n          endAt\n          service {\n            name\n            logoUrl\n            channelType {\n              type\n            }\n            id\n          }\n        }\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "9748ef10c0c0d41a3f631e5166e05e34";

export default node;
