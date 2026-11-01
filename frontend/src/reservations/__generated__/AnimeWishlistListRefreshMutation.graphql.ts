/**
 * @generated SignedSource<<c692ca1fb06fb6abafaf6147b9cfd79a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeWishlistListRefreshMutation$variables = Record<PropertyKey, never>;
export type AnimeWishlistListRefreshMutation$data = {
  readonly refreshAnimeWishlist: boolean;
};
export type AnimeWishlistListRefreshMutation = {
  response: AnimeWishlistListRefreshMutation$data;
  variables: AnimeWishlistListRefreshMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "refreshAnimeWishlist",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "AnimeWishlistListRefreshMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "AnimeWishlistListRefreshMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "b04e36bcbc1d15a2308c7323b8a01073",
    "id": null,
    "metadata": {},
    "name": "AnimeWishlistListRefreshMutation",
    "operationKind": "mutation",
    "text": "mutation AnimeWishlistListRefreshMutation {\n  refreshAnimeWishlist\n}\n"
  }
};
})();

(node as any).hash = "910c60f526d5286d17b2ff07e900b739";

export default node;
