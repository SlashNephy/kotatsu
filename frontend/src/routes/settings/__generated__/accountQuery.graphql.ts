/**
 * @generated SignedSource<<82229907c57972c535e802845ff1ef42>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type accountQuery$variables = Record<PropertyKey, never>;
export type accountQuery$data = {
  readonly animeRecordSettings: {
    readonly destinations: ReadonlyArray<{
      readonly available: boolean;
      readonly connection: {
        readonly accountName: string;
        readonly connectedAt: string;
      } | null | undefined;
      readonly destination: AnimeRecordDestination;
    }>;
    readonly enabled: boolean;
    readonly notifyWishlist: boolean;
    readonly thresholdPercent: number;
    readonly watchedOnLast: boolean;
    readonly watchingOnEpisode: boolean;
    readonly watchingOnFirst: boolean;
  };
};
export type accountQuery = {
  response: accountQuery$data;
  variables: accountQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
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
        "kind": "ScalarField",
        "name": "enabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "thresholdPercent",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "watchingOnFirst",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "watchingOnEpisode",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "watchedOnLast",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "notifyWishlist",
        "storageKey": null
      },
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
            "kind": "ScalarField",
            "name": "destination",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "available",
            "storageKey": null
          },
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
                "name": "accountName",
                "storageKey": null
              },
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
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "accountQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "accountQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "efe89edc4ed5233519298daf16de8664",
    "id": null,
    "metadata": {},
    "name": "accountQuery",
    "operationKind": "query",
    "text": "query accountQuery {\n  animeRecordSettings {\n    enabled\n    thresholdPercent\n    watchingOnFirst\n    watchingOnEpisode\n    watchedOnLast\n    notifyWishlist\n    destinations {\n      destination\n      available\n      connection {\n        accountName\n        connectedAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "17d5898e8b261fde40091692ba4e6412";

export default node;
