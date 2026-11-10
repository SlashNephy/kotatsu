/**
 * @generated SignedSource<<1d9f127ddbf3c4e53d5acee065392b18>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type usersDeleteMutation$variables = {
  id: string;
};
export type usersDeleteMutation$data = {
  readonly deleteUser: string;
};
export type usersDeleteMutation = {
  response: usersDeleteMutation$data;
  variables: usersDeleteMutation$variables;
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
    "kind": "ScalarField",
    "name": "deleteUser",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "usersDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "usersDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "155b3b1d8961010eb84d624bc8a515b0",
    "id": null,
    "metadata": {},
    "name": "usersDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation usersDeleteMutation(\n  $id: ID!\n) {\n  deleteUser(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "64982d3d98d1973d88fb3a9b85af43f6";

export default node;
