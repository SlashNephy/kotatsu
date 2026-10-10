/**
 * @generated SignedSource<<7cd7a8c099b5e30caba5948ceaabf60c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ProgramDetailReserveMutation$variables = {
  program: string;
};
export type ProgramDetailReserveMutation$data = {
  readonly createProgramReservation: {
    readonly id: string;
  };
};
export type ProgramDetailReserveMutation = {
  response: ProgramDetailReserveMutation$data;
  variables: ProgramDetailReserveMutation$variables;
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
    "name": "ProgramDetailReserveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ProgramDetailReserveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "b770ece3a3e964bccdbbaf3cdb8e13e0",
    "id": null,
    "metadata": {},
    "name": "ProgramDetailReserveMutation",
    "operationKind": "mutation",
    "text": "mutation ProgramDetailReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "497bdf449e535f4977d429c22f4c84cb";

export default node;
