/**
 * @generated SignedSource<<cf1279fe7cb029c004097826a24aefd9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type reservationsWishlistTabQuery$variables = Record<PropertyKey, never>;
export type reservationsWishlistTabQuery$data = {
  readonly animeRecordSettings: {
    readonly enabled: boolean;
  };
};
export type reservationsWishlistTabQuery = {
  response: reservationsWishlistTabQuery$data;
  variables: reservationsWishlistTabQuery$variables;
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
    "name": "reservationsWishlistTabQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "reservationsWishlistTabQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "12b96a9bac30d50fbe588b4ec1718cc6",
    "id": null,
    "metadata": {},
    "name": "reservationsWishlistTabQuery",
    "operationKind": "query",
    "text": "query reservationsWishlistTabQuery {\n  animeRecordSettings {\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "18717acec7df36d3ce6c0c3ed05731c6";

export default node;
