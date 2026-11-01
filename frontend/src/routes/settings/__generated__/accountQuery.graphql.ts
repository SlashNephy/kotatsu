/**
 * @generated SignedSource<<f2827b88635a80510ac0ef023a2428c0>>
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
    "cacheID": "d63dc7675336c7951535954cae3b797b",
    "id": null,
    "metadata": {},
    "name": "accountQuery",
    "operationKind": "query",
    "text": "query accountQuery {\n  animeRecordSettings {\n    enabled\n    thresholdPercent\n    watchingOnFirst\n    watchingOnEpisode\n    watchedOnLast\n    destinations {\n      destination\n      available\n      connection {\n        accountName\n        connectedAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "3af087c166825cce33c5a8e28d03c770";

export default node;
