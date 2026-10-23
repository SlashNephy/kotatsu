/**
 * @generated SignedSource<<181ca4596fc7e3d8055a3f3ff3185bef>>
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
export type systemPoolMutation$variables = {
  id: string;
  input: StoragePoolInput;
};
export type systemPoolMutation$data = {
  readonly updateStoragePool: {
    readonly id: string;
    readonly minFreeBytes: number;
    readonly priority: number;
  };
};
export type systemPoolMutation = {
  response: systemPoolMutation$data;
  variables: systemPoolMutation$variables;
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
    "name": "systemPoolMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "systemPoolMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "22bb978767e7e1293be8bb89b20b1f10",
    "id": null,
    "metadata": {},
    "name": "systemPoolMutation",
    "operationKind": "mutation",
    "text": "mutation systemPoolMutation(\n  $id: String!\n  $input: StoragePoolInput!\n) {\n  updateStoragePool(id: $id, input: $input) {\n    id\n    priority\n    minFreeBytes\n  }\n}\n"
  }
};
})();

(node as any).hash = "7e484925c933a8a261e18c0529e6afa7";

export default node;
