/**
 * @generated SignedSource<<205d9e7273d37eebff5d43ead9b982d0>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ProgramDetailCancelMutation$variables = {
  id: string;
};
export type ProgramDetailCancelMutation$data = {
  readonly deleteReservation: {
    readonly id: string;
    readonly reservation: {
      readonly enabled: boolean;
      readonly id: string;
    } | null | undefined;
  };
};
export type ProgramDetailCancelMutation = {
  response: ProgramDetailCancelMutation$data;
  variables: ProgramDetailCancelMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      }
    ],
    "concreteType": "DeleteReservationPayload",
    "kind": "LinkedField",
    "name": "deleteReservation",
    "plural": false,
    "selections": [
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "Reservation",
        "kind": "LinkedField",
        "name": "reservation",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
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
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ProgramDetailCancelMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ProgramDetailCancelMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "6265b95ca8aa4bcc4be92cdc2750a853",
    "id": null,
    "metadata": {},
    "name": "ProgramDetailCancelMutation",
    "operationKind": "mutation",
    "text": "mutation ProgramDetailCancelMutation(\n  $id: ID!\n) {\n  deleteReservation(id: $id) {\n    id\n    reservation {\n      id\n      enabled\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "ce5feba6e91349663e3fedb7df7d055d";

export default node;
