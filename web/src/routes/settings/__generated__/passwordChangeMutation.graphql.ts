/**
 * @generated SignedSource<<ca9d5c122c7a3d2baaadef6eb739575a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type passwordChangeMutation$variables = {
  password: string;
};
export type passwordChangeMutation$data = {
  readonly changePassword: boolean;
};
export type passwordChangeMutation = {
  response: passwordChangeMutation$data;
  variables: passwordChangeMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
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
        "name": "password",
        "variableName": "password"
      }
    ],
    "kind": "ScalarField",
    "name": "changePassword",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "passwordChangeMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "passwordChangeMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "d9691703a2e695a2fc73e48c317e350f",
    "id": null,
    "metadata": {},
    "name": "passwordChangeMutation",
    "operationKind": "mutation",
    "text": "mutation passwordChangeMutation(\n  $password: String!\n) {\n  changePassword(password: $password)\n}\n"
  }
};
})();

(node as any).hash = "ebc10be347a33198a2c0266c8d46a09f";

export default node;
