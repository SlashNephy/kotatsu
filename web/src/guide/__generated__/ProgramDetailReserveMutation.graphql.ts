/**
 * @generated SignedSource<<236db3d8385bd63f30d07861f2ee284f>>
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
    readonly enabled: boolean;
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
      },
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
    "cacheID": "e00e6a06f3d1b944e2a112abd01876af",
    "id": null,
    "metadata": {},
    "name": "ProgramDetailReserveMutation",
    "operationKind": "mutation",
    "text": "mutation ProgramDetailReserveMutation(\n  $program: ID!\n) {\n  createProgramReservation(input: {program: $program}) {\n    id\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "d4415b9190043e1ff1c0bd995b68a877";

export default node;
