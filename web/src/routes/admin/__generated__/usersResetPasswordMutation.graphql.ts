/**
 * @generated SignedSource<<0fea0d043f3eb3a5453fdc2cb4ef7e7d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type usersResetPasswordMutation$variables = {
  id: string;
  password: string;
};
export type usersResetPasswordMutation$data = {
  readonly resetUserPassword: {
    readonly id: string;
  };
};
export type usersResetPasswordMutation = {
  response: usersResetPasswordMutation$data;
  variables: usersResetPasswordMutation$variables;
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
    "name": "password"
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
        "name": "password",
        "variableName": "password"
      }
    ],
    "concreteType": "User",
    "kind": "LinkedField",
    "name": "resetUserPassword",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
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
    "name": "usersResetPasswordMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "usersResetPasswordMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "1cbad99b030edfd1e04152d34d8dc179",
    "id": null,
    "metadata": {},
    "name": "usersResetPasswordMutation",
    "operationKind": "mutation",
    "text": "mutation usersResetPasswordMutation(\n  $id: ID!\n  $password: String!\n) {\n  resetUserPassword(id: $id, password: $password) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "3fdf10ae72c7810a5ce609a827e291f3";

export default node;
