/**
 * @generated SignedSource<<127d01b2d166e47eec5888a6bd1168e2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type reservationsSetManyEnabledMutation$variables = {
  enabled: boolean;
  ids: ReadonlyArray<string>;
};
export type reservationsSetManyEnabledMutation$data = {
  readonly setReservationsEnabled: {
    readonly processed: number;
    readonly skipped: number;
  };
};
export type reservationsSetManyEnabledMutation = {
  response: reservationsSetManyEnabledMutation$data;
  variables: reservationsSetManyEnabledMutation$variables;
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
  "name": "ids"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "enabled",
        "variableName": "enabled"
      },
      {
        "kind": "Variable",
        "name": "ids",
        "variableName": "ids"
      }
    ],
    "concreteType": "BulkOperationPayload",
    "kind": "LinkedField",
    "name": "setReservationsEnabled",
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
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "reservationsSetManyEnabledMutation",
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
    "name": "reservationsSetManyEnabledMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "f61cfe648c02c8cd74c12c4ee5b5c7c5",
    "id": null,
    "metadata": {},
    "name": "reservationsSetManyEnabledMutation",
    "operationKind": "mutation",
    "text": "mutation reservationsSetManyEnabledMutation(\n  $ids: [ID!]!\n  $enabled: Boolean!\n) {\n  setReservationsEnabled(ids: $ids, enabled: $enabled) {\n    processed\n    skipped\n  }\n}\n"
  }
};
})();

(node as any).hash = "a93488fc593f5e10100d48d7f282476e";

export default node;
