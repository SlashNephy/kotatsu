/**
 * @generated SignedSource<<8eba13d83d69e8b8844e2e6a442de178>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type OAuthGrantsDeleteMutation$variables = {
  id: string;
};
export type OAuthGrantsDeleteMutation$data = {
  readonly deleteOAuthGrant: string;
};
export type OAuthGrantsDeleteMutation = {
  response: OAuthGrantsDeleteMutation$data;
  variables: OAuthGrantsDeleteMutation$variables;
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
    "name": "deleteOAuthGrant",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "OAuthGrantsDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "OAuthGrantsDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ea244b5bf51a67243ceff3aaa0129028",
    "id": null,
    "metadata": {},
    "name": "OAuthGrantsDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation OAuthGrantsDeleteMutation(\n  $id: ID!\n) {\n  deleteOAuthGrant(id: $id)\n}\n"
  }
};
})();

(node as any).hash = "72f1cfc58b6a63ce6354fa4dbd25e1f0";

export default node;
