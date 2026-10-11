/**
 * @generated SignedSource<<f50878267069e92dd31ade7ab4e50b59>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type accountRevokeSessionMutation$variables = {
  id: string;
};
export type accountRevokeSessionMutation$data = {
  readonly revokeSession: string;
};
export type accountRevokeSessionMutation = {
  response: accountRevokeSessionMutation$data;
  variables: accountRevokeSessionMutation$variables;
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
    "name": "revokeSession",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "accountRevokeSessionMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "accountRevokeSessionMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ef24bc8563603a194458a023e2b66bde",
    "id": null,
    "metadata": {},
    "name": "accountRevokeSessionMutation",
    "operationKind": "mutation",
    "text": "mutation accountRevokeSessionMutation(\n  $id: ID!\n) {\n  revokeSession(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "d6be8e649123226ae333d5c51d076198";

export default node;
