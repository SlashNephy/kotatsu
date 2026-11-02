/**
 * @generated SignedSource<<837acb3c346a8517dad5cf8a6bd18545>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeLibraryReserveMutation$variables = {
  program: string;
};
export type AnimeLibraryReserveMutation$data = {
  readonly createProgramReservation: {
    readonly id: string;
  };
};
export type AnimeLibraryReserveMutation = {
  response: AnimeLibraryReserveMutation$data;
  variables: AnimeLibraryReserveMutation$variables;
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
    "name": "AnimeLibraryReserveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "AnimeLibraryReserveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "c028d2421adcd226a82eeca51aeba376",
    "id": null,
    "metadata": {},
    "name": "AnimeLibraryReserveMutation",
    "operationKind": "mutation",
    "text": "mutation AnimeLibraryReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "2fe5ddab03380f50a173c2051d917a8a";

export default node;
