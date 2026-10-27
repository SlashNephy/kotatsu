/**
 * @generated SignedSource<<0968ec7b98205ebbfc0bff0e17ff624b>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type StoragePoolInput = {
  minFreeBytes?: number | null | undefined;
  priority?: number | null | undefined;
};
export type storagePoolMutation$variables = {
  id: string;
  input: StoragePoolInput;
};
export type storagePoolMutation$data = {
  readonly updateStoragePool: {
    readonly id: string;
    readonly minFreeBytes: number;
    readonly priority: number;
  };
};
export type storagePoolMutation = {
  response: storagePoolMutation$data;
  variables: storagePoolMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      },
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "StoragePool",
    "kind": "LinkedField",
    "name": "updateStoragePool",
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
        "name": "priority",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "minFreeBytes",
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
    "name": "storagePoolMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "storagePoolMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "925e3ff7806c0d121c9d93350422ccf5",
    "id": null,
    "metadata": {},
    "name": "storagePoolMutation",
    "operationKind": "mutation",
    "text": "mutation storagePoolMutation(\n  $id: String!\n  $input: StoragePoolInput!\n) {\n  updateStoragePool(id: $id, input: $input) {\n    id\n    priority\n    minFreeBytes\n  }\n}\n"
  }
};
})();

(node as any).hash = "8f8089acc4a189b28dcf8b52d01a6863";

export default node;
