/**
 * @generated SignedSource<<b0b1e6fb94c2d8c0df0df75d8d3ca1f2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type reservationsEnableMutation$variables = {
  enabled: boolean;
  id: string;
};
export type reservationsEnableMutation$data = {
  readonly updateReservation: {
    readonly enabled: boolean;
    readonly id: string;
  };
};
export type reservationsEnableMutation = {
  response: reservationsEnableMutation$data;
  variables: reservationsEnableMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "enabled"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "id"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      },
      {
        "fields": [
          {
            "kind": "Variable",
            "name": "enabled",
            "variableName": "enabled"
          }
        ],
        "kind": "ObjectValue",
        "name": "input"
      }
    ],
    "concreteType": "Reservation",
    "kind": "LinkedField",
    "name": "updateReservation",
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
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "reservationsEnableMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "reservationsEnableMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "b678b12cdbb10a073c79e810e4f47b76",
    "id": null,
    "metadata": {},
    "name": "reservationsEnableMutation",
    "operationKind": "mutation",
    "text": "mutation reservationsEnableMutation(\n  $id: ID!\n  $enabled: Boolean!\n) {\n  updateReservation(id: $id, input: {enabled: $enabled}) {\n    id\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "ccfae06342f6caea2737fc6987a1f77d";

export default node;
