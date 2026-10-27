/**
 * @generated SignedSource<<eec4f0d0a443f8b843cd6c9fa8599584>>
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
export type servicesServiceMutation$variables = {
  id: string;
  input: ServiceInput;
};
export type servicesServiceMutation$data = {
  readonly updateService: {
    readonly hidden: boolean;
    readonly id: string;
    readonly sortOrder: number | null | undefined;
  };
};
export type servicesServiceMutation = {
  response: servicesServiceMutation$data;
  variables: servicesServiceMutation$variables;
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
    "name": "servicesServiceMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "servicesServiceMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ffae66ce3076bbd254b3a05400dea258",
    "id": null,
    "metadata": {},
    "name": "servicesServiceMutation",
    "operationKind": "mutation",
    "text": "mutation servicesServiceMutation(\n  $id: ID!\n  $input: ServiceInput!\n) {\n  updateService(id: $id, input: $input) {\n    id\n    hidden\n    sortOrder\n  }\n}\n"
  }
};
})();

(node as any).hash = "3247f78f9a64a3b6f001c740dc603984";

export default node;
