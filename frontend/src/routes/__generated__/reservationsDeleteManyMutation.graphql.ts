/**
 * @generated SignedSource<<60bf6cbcc3a4e8f4a5135ecee2059cdc>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type reservationsDeleteManyMutation$variables = {
  ids: ReadonlyArray<string>;
};
export type reservationsDeleteManyMutation$data = {
  readonly deleteReservations: {
    readonly processed: number;
    readonly skipped: number;
  };
};
export type reservationsDeleteManyMutation = {
  response: reservationsDeleteManyMutation$data;
  variables: reservationsDeleteManyMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "ids"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "ids",
        "variableName": "ids"
      }
    ],
    "concreteType": "BulkOperationPayload",
    "kind": "LinkedField",
    "name": "deleteReservations",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "processed",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "skipped",
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
    "name": "reservationsDeleteManyMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "reservationsDeleteManyMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "8715b47ea63f5c627ee3026cf45ad4b1",
    "id": null,
    "metadata": {},
    "name": "reservationsDeleteManyMutation",
    "operationKind": "mutation",
    "text": "mutation reservationsDeleteManyMutation(\n  $ids: [ID!]!\n) {\n  deleteReservations(ids: $ids) {\n    processed\n    skipped\n  }\n}\n"
  }
};
})();

(node as any).hash = "a8418379834d6c9cf8e48b08e1903308";

export default node;
