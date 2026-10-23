/**
 * @generated SignedSource<<352613d296b1de3a096e08afbbc4c223>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type CreateUserInput = {
  displayName: string;
  password?: string | null | undefined;
  role: Role;
  username: string;
};
export type usersCreateMutation$variables = {
  input: CreateUserInput;
};
export type usersCreateMutation$data = {
  readonly createUser: {
    readonly id: string;
    readonly username: string;
  };
};
export type usersCreateMutation = {
  response: usersCreateMutation$data;
  variables: usersCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
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
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "User",
    "kind": "LinkedField",
    "name": "createUser",
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
        "name": "username",
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
    "name": "usersCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "usersCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "2ebf411c34fe251ad4e4aafb8781013f",
    "id": null,
    "metadata": {},
    "name": "usersCreateMutation",
    "operationKind": "mutation",
    "text": "mutation usersCreateMutation(\n  $input: CreateUserInput!\n) {\n  createUser(input: $input) {\n    id\n    username\n  }\n}\n"
  }
};
})();

(node as any).hash = "7eaf9fa563f5f32100da787c005cb7ec";

export default node;
