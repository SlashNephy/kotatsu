/**
 * @generated SignedSource<<ba063937934e894d6c2d24ffaa22effa>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeWishlistListCreateRuleMutation$variables = {
  tid: number;
};
export type AnimeWishlistListCreateRuleMutation$data = {
  readonly createAnimeWishlistRule: {
    readonly id: string;
    readonly name: string;
  };
};
export type AnimeWishlistListCreateRuleMutation = {
  response: AnimeWishlistListCreateRuleMutation$data;
  variables: AnimeWishlistListCreateRuleMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "tid"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "tid",
        "variableName": "tid"
      }
    ],
    "concreteType": "Rule",
    "kind": "LinkedField",
    "name": "createAnimeWishlistRule",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "name",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "AnimeWishlistListCreateRuleMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "AnimeWishlistListCreateRuleMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "12256d3f6872567992762b01d0209c9b",
    "id": null,
    "metadata": {},
    "name": "AnimeWishlistListCreateRuleMutation",
    "operationKind": "mutation",
    "text": "mutation AnimeWishlistListCreateRuleMutation(\n  $tid: Int!\n) {\n  createAnimeWishlistRule(tid: $tid) {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "d3625714a5a3053a11f92ca209fa4e18";

export default node;
