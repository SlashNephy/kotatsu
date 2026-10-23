/**
 * @generated SignedSource<<eed1d1e0df291c8b822df3f09f74d2e3>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ApiTokensDeleteMutation$variables = {
  id: string;
};
export type ApiTokensDeleteMutation$data = {
  readonly deleteApiToken: string;
};
export type ApiTokensDeleteMutation = {
  response: ApiTokensDeleteMutation$data;
  variables: ApiTokensDeleteMutation$variables;
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
    "name": "deleteApiToken",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ApiTokensDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ApiTokensDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "798c0528b1fe9151983ed770476d28fc",
    "id": null,
    "metadata": {},
    "name": "ApiTokensDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation ApiTokensDeleteMutation(\n  $id: ID!\n) {\n  deleteApiToken(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "66cdccc960c00d2bb9fda24d0ed1607e";

export default node;
