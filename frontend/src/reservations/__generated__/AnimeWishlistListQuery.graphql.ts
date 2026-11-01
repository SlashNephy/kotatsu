/**
 * @generated SignedSource<<c23746e6101090754260ccb38e10eb8f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type AnimeWishlistStatus = "WANNA_WATCH" | "WATCHING" | "%future added value";
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type AnimeWishlistListQuery$variables = Record<PropertyKey, never>;
export type AnimeWishlistListQuery$data = {
  readonly animeWishlist: {
    readonly enabled: boolean;
    readonly imports: ReadonlyArray<{
      readonly destination: AnimeRecordDestination;
      readonly entryCount: number;
      readonly error: string | null | undefined;
      readonly importedAt: string | null | undefined;
    }>;
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
      readonly sources: ReadonlyArray<{
        readonly destination: AnimeRecordDestination;
        readonly status: AnimeWishlistStatus;
        readonly url: string;
        readonly workId: number;
      }>;
      readonly tid: number;
      readonly title: string;
    }>;
  };
  readonly viewer: {
    readonly role: Role;
  };
};
export type AnimeWishlistListQuery = {
  response: AnimeWishlistListQuery$data;
  variables: AnimeWishlistListQuery$variables;
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
  "name": "tid",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "title",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "reservedBroadcasts",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "covered",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "concreteType": "AnimeWishlistSource",
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
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "status",
      "storageKey": null
    },
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
  "name": "name",
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
    "name": "AnimeWishlistListQuery",
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
        "concreteType": "AnimeWishlist",
        "kind": "LinkedField",
        "name": "animeWishlist",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "AnimeWishlistWork",
            "kind": "LinkedField",
            "name": "works",
            "plural": true,
            "selections": [
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
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
                      (v11/*:: as any*/),
                      (v12/*:: as any*/),
                      (v13/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Service",
                        "kind": "LinkedField",
                        "name": "service",
                        "plural": false,
                        "selections": [
                          (v11/*:: as any*/),
                          (v14/*:: as any*/),
                          (v15/*:: as any*/)
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
    "name": "AnimeWishlistListQuery",
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
      {
        "alias": null,
        "args": null,
        "concreteType": "AnimeWishlist",
        "kind": "LinkedField",
        "name": "animeWishlist",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "AnimeWishlistWork",
            "kind": "LinkedField",
            "name": "works",
            "plural": true,
            "selections": [
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
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
                      (v11/*:: as any*/),
                      (v12/*:: as any*/),
                      (v13/*:: as any*/),
                      {
                        "alias": null,
                        "args": null,
                        "concreteType": "Service",
                        "kind": "LinkedField",
                        "name": "service",
                        "plural": false,
                        "selections": [
                          (v11/*:: as any*/),
                          (v14/*:: as any*/),
                          (v15/*:: as any*/),
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
    "cacheID": "c17dfa4a0818203830698deba848bc54",
    "id": null,
    "metadata": {},
    "name": "AnimeWishlistListQuery",
    "operationKind": "query",
    "text": "query AnimeWishlistListQuery {\n  viewer {\n    role\n    id\n  }\n  animeWishlist {\n    enabled\n    imports {\n      destination\n      importedAt\n      error\n      entryCount\n    }\n    works {\n      tid\n      title\n      reservedBroadcasts\n      covered\n      sources {\n        destination\n        workId\n        status\n        url\n      }\n      broadcasts {\n        preferred\n        program {\n          id\n          name\n          startAt\n          endAt\n          service {\n            name\n            logoUrl\n            channelType {\n              type\n            }\n            id\n          }\n        }\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "67732e254f4376846ad6093018f50e78";

export default node;
