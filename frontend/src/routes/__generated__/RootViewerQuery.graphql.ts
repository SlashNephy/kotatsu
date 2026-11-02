/**
 * @generated SignedSource<<d2de123ac978381f55e33456d48e33a2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RootViewerQuery$variables = Record<PropertyKey, never>;
export type RootViewerQuery$data = {
  readonly animeRecordSettings: {
    readonly enabled: boolean;
  };
  readonly viewer: {
    readonly username: string;
  };
};
export type RootViewerQuery = {
  response: RootViewerQuery$data;
  variables: RootViewerQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "username",
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
      "kind": "ScalarField",
      "name": "enabled",
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
    "name": "RootViewerQuery",
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
      (v1/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "RootViewerQuery",
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
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "id",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v1/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "e7f5fbd427c09f5f4e8ab4f13a9cf60b",
    "id": null,
    "metadata": {},
    "name": "RootViewerQuery",
    "operationKind": "query",
    "text": "query RootViewerQuery {\n  viewer {\n    username\n    id\n  }\n  animeRecordSettings {\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "ce6a2b751c0104594110db86e2056995";

export default node;
