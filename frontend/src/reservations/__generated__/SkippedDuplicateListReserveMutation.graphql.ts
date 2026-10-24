/**
 * @generated SignedSource<<03fe89cef7a2de695c5d65d5ae6ab5d0>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type SkippedDuplicateListReserveMutation$variables = {
  program: string;
};
export type SkippedDuplicateListReserveMutation$data = {
  readonly createProgramReservation: {
    readonly id: string;
  };
};
export type SkippedDuplicateListReserveMutation = {
  response: SkippedDuplicateListReserveMutation$data;
  variables: SkippedDuplicateListReserveMutation$variables;
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
    "name": "SkippedDuplicateListReserveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "SkippedDuplicateListReserveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f36c6ba68a3e6aac2c1cf8a46dc34698",
    "id": null,
    "metadata": {},
    "name": "SkippedDuplicateListReserveMutation",
    "operationKind": "mutation",
    "text": "mutation SkippedDuplicateListReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "e340ae0a585e217ec80852fc9607fdbc";

export default node;
