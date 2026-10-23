/**
 * @generated SignedSource<<ce230788978e2c4db4e71101eb26e926>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type UpdateUserInput = {
  displayName?: string | null | undefined;
  role?: Role | null | undefined;
};
export type usersUpdateMutation$variables = {
  id: string;
  input: UpdateUserInput;
};
export type usersUpdateMutation$data = {
  readonly updateUser: {
    readonly displayName: string;
    readonly id: string;
    readonly role: Role;
  };
};
export type usersUpdateMutation = {
  response: usersUpdateMutation$data;
  variables: usersUpdateMutation$variables;
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
    "concreteType": "User",
    "kind": "LinkedField",
    "name": "updateUser",
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
        "name": "displayName",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "role",
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
    "name": "usersUpdateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "usersUpdateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "867a240f867d3813d5835c8e2e5e2572",
    "id": null,
    "metadata": {},
    "name": "usersUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation usersUpdateMutation(\n  $id: ID!\n  $input: UpdateUserInput!\n) {\n  updateUser(id: $id, input: $input) {\n    id\n    displayName\n    role\n  }\n}\n"
  }
};
})();

(node as any).hash = "5cb4dda90010cd59c1abbe816bc49359";

export default node;
