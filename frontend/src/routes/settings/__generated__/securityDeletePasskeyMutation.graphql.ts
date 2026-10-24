/**
 * @generated SignedSource<<bc2a797de9b9a160644f1b93e51ada9e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type securityDeletePasskeyMutation$variables = {
  id: string;
};
export type securityDeletePasskeyMutation$data = {
  readonly deletePasskey: string;
};
export type securityDeletePasskeyMutation = {
  response: securityDeletePasskeyMutation$data;
  variables: securityDeletePasskeyMutation$variables;
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
    "name": "deletePasskey",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "securityDeletePasskeyMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "securityDeletePasskeyMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f29f713bcd41aa15c13e2beed94bda42",
    "id": null,
    "metadata": {},
    "name": "securityDeletePasskeyMutation",
    "operationKind": "mutation",
    "text": "mutation securityDeletePasskeyMutation(\n  $id: ID!\n) {\n  deletePasskey(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "fd931332493df2e24ce69ec5aaf90867";

export default node;
