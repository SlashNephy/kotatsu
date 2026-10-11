/**
 * @generated SignedSource<<a6ae61f91fcd00c1d2d91ccd8f7adc9e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type usersEnableMutation$variables = {
  id: string;
};
export type usersEnableMutation$data = {
  readonly enableUser: {
    readonly disabled: boolean;
    readonly id: string;
  };
};
export type usersEnableMutation = {
  response: usersEnableMutation$data;
  variables: usersEnableMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
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
      }
    ],
    "concreteType": "User",
    "kind": "LinkedField",
    "name": "enableUser",
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
        "name": "disabled",
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
    "name": "usersEnableMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "usersEnableMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "6e7728f78dc9f4fbbf153377827e296f",
    "id": null,
    "metadata": {},
    "name": "usersEnableMutation",
    "operationKind": "mutation",
    "text": "mutation usersEnableMutation(\n  $id: ID!\n) {\n  enableUser(id: $id) {\n    id\n    disabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "67a4deb1013d5cea26142b32035f110e";

export default node;
