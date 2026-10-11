/**
 * @generated SignedSource<<2baedc19a5da07bc2de3698c576412e4>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type usersDisableMutation$variables = {
  id: string;
};
export type usersDisableMutation$data = {
  readonly disableUser: {
    readonly disabled: boolean;
    readonly id: string;
  };
};
export type usersDisableMutation = {
  response: usersDisableMutation$data;
  variables: usersDisableMutation$variables;
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
    "name": "disableUser",
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
    "name": "usersDisableMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "usersDisableMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "b8d961008218b9050408ea77428d4ef1",
    "id": null,
    "metadata": {},
    "name": "usersDisableMutation",
    "operationKind": "mutation",
    "text": "mutation usersDisableMutation(\n  $id: ID!\n) {\n  disableUser(id: $id) {\n    id\n    disabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "0f11ef4bea51bc3a056a76a703b759cf";

export default node;
