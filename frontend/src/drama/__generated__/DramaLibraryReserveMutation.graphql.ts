/**
 * @generated SignedSource<<261540e50629d22ba6b2c63475b26807>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaLibraryReserveMutation$variables = {
  program: string;
};
export type DramaLibraryReserveMutation$data = {
  readonly createProgramReservation: {
    readonly id: string;
  };
};
export type DramaLibraryReserveMutation = {
  response: DramaLibraryReserveMutation$data;
  variables: DramaLibraryReserveMutation$variables;
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
    "name": "DramaLibraryReserveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "DramaLibraryReserveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "9612c97131cf9b96192bcb6dc98199b6",
    "id": null,
    "metadata": {},
    "name": "DramaLibraryReserveMutation",
    "operationKind": "mutation",
    "text": "mutation DramaLibraryReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "b7c983a55c1b1d7867fc553ef33ca291";

export default node;
