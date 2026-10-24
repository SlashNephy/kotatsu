/**
 * @generated SignedSource<<b181f41b996e0a2788e29fb330cd089f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type authorizeDenyMutation$variables = {
  query: string;
};
export type authorizeDenyMutation$data = {
  readonly denyOAuthAuthorization: string;
};
export type authorizeDenyMutation = {
  response: authorizeDenyMutation$data;
  variables: authorizeDenyMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "query"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "query",
        "variableName": "query"
      }
    ],
    "kind": "ScalarField",
    "name": "denyOAuthAuthorization",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "authorizeDenyMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "authorizeDenyMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "cb145d3b4dcb7e10b390587f6dc591ff",
    "id": null,
    "metadata": {},
    "name": "authorizeDenyMutation",
    "operationKind": "mutation",
    "text": "mutation authorizeDenyMutation(\n  $query: String!\n) {\n  denyOAuthAuthorization(query: $query)\n}\n"
  }
};
})();

(node as any).hash = "9d9e2a9981d035e6cbc491aaf4c977e5";

export default node;
