/**
 * @generated SignedSource<<ef69a7b50f60b3c8def2d76cfbbfbc3a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ServiceInput = {
  hidden?: boolean | null | undefined;
  resetSortOrder?: boolean;
  sortOrder?: number | null | undefined;
};
export type systemServiceMutation$variables = {
  id: string;
  input: ServiceInput;
};
export type systemServiceMutation$data = {
  readonly updateService: {
    readonly hidden: boolean;
    readonly id: string;
    readonly sortOrder: number | null | undefined;
  };
};
export type systemServiceMutation = {
  response: systemServiceMutation$data;
  variables: systemServiceMutation$variables;
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
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "updateService",
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
        "name": "hidden",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "sortOrder",
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
    "name": "systemServiceMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "systemServiceMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ac2ad22c635d751da9fb10ba1168e8b1",
    "id": null,
    "metadata": {},
    "name": "systemServiceMutation",
    "operationKind": "mutation",
    "text": "mutation systemServiceMutation(\n  $id: ID!\n  $input: ServiceInput!\n) {\n  updateService(id: $id, input: $input) {\n    id\n    hidden\n    sortOrder\n  }\n}\n"
  }
};
})();

(node as any).hash = "d85020b683f6414425b46f5c387821c6";

export default node;
