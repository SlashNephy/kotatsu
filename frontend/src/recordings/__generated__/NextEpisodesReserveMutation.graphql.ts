/**
 * @generated SignedSource<<d25a481a481118f5dc233991b54c92e3>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NextEpisodesReserveMutation$variables = {
  program: string;
};
export type NextEpisodesReserveMutation$data = {
  readonly createProgramReservation: {
    readonly id: string;
  };
};
export type NextEpisodesReserveMutation = {
  response: NextEpisodesReserveMutation$data;
  variables: NextEpisodesReserveMutation$variables;
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
    "name": "NextEpisodesReserveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "NextEpisodesReserveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "d9f227dd8b1d5760261637af17bfaaf9",
    "id": null,
    "metadata": {},
    "name": "NextEpisodesReserveMutation",
    "operationKind": "mutation",
    "text": "mutation NextEpisodesReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "d102af0375b21cc60816e5c263693d2b";

export default node;
