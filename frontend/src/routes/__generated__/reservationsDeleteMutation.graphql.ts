/**
 * @generated SignedSource<<a4d1194d6ebe0566015e8bcf2d88eadf>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type reservationsDeleteMutation$variables = {
  id: string;
};
export type reservationsDeleteMutation$data = {
  readonly deleteReservation: {
    readonly id: string;
    readonly reservation: {
      readonly enabled: boolean;
      readonly id: string;
    } | null | undefined;
  };
};
export type reservationsDeleteMutation = {
  response: reservationsDeleteMutation$data;
  variables: reservationsDeleteMutation$variables;
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
    "name": "reservationsDeleteMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "reservationsDeleteMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "741eda5f3f4f064c86421b4784acd753",
    "id": null,
    "metadata": {},
    "name": "reservationsDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation reservationsDeleteMutation(\n  $id: ID!\n) {\n  deleteReservation(id: $id) {\n    id\n    reservation {\n      id\n      enabled\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "f66bf650c9ee3126ad4e2aca24296bb9";

export default node;
