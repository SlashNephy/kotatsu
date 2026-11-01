/**
 * @generated SignedSource<<a4ac8c085b2103feaf8614cc450a387f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeWishlistListReserveMutation$variables = {
  program: string;
};
export type AnimeWishlistListReserveMutation$data = {
  readonly createProgramReservation: {
    readonly id: string;
  };
};
export type AnimeWishlistListReserveMutation = {
  response: AnimeWishlistListReserveMutation$data;
  variables: AnimeWishlistListReserveMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "program"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "fields": [
          {
            "kind": "Variable",
            "name": "program",
            "variableName": "program"
          }
        ],
        "kind": "ObjectValue",
        "name": "input"
      }
    ],
    "concreteType": "Reservation",
    "kind": "LinkedField",
    "name": "createProgramReservation",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
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
    "name": "AnimeWishlistListReserveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "AnimeWishlistListReserveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "11fd03eae7a0288c2cf13c07f96372fa",
    "id": null,
    "metadata": {},
    "name": "AnimeWishlistListReserveMutation",
    "operationKind": "mutation",
    "text": "mutation AnimeWishlistListReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "0c6906cd1f81775c5673f3a5bb6f6c98";

export default node;
