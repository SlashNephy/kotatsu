/**
 * @generated SignedSource<<f504e263c2f9af7c4df4b6dd7ef2addf>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeLibraryCreateRuleMutation$variables = {
  tid: number;
};
export type AnimeLibraryCreateRuleMutation$data = {
  readonly createAnimeWishlistRule: {
    readonly id: string;
    readonly name: string;
  };
};
export type AnimeLibraryCreateRuleMutation = {
  response: AnimeLibraryCreateRuleMutation$data;
  variables: AnimeLibraryCreateRuleMutation$variables;
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
    "name": "AnimeLibraryCreateRuleMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "AnimeLibraryCreateRuleMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "4e3dab090848352ecb2a7ece175899e4",
    "id": null,
    "metadata": {},
    "name": "AnimeLibraryCreateRuleMutation",
    "operationKind": "mutation",
    "text": "mutation AnimeLibraryCreateRuleMutation(\n  $tid: Int!\n) {\n  createAnimeWishlistRule(tid: $tid) {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "44a2332b85c6743eed4f68f7a067f00f";

export default node;
