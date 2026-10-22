/**
 * @generated SignedSource<<71321c4a0590fc97755ac5147389973a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type authorizeApproveMutation$variables = {
  query: string;
  scopes: ReadonlyArray<string>;
};
export type authorizeApproveMutation$data = {
  readonly approveOAuthAuthorization: string;
};
export type authorizeApproveMutation = {
  response: authorizeApproveMutation$data;
  variables: authorizeApproveMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "query"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "scopes"
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
      },
      {
        "kind": "Variable",
        "name": "scopes",
        "variableName": "scopes"
      }
    ],
    "kind": "ScalarField",
    "name": "approveOAuthAuthorization",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "authorizeApproveMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "authorizeApproveMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "37158565143a88201e3bc46597b509e6",
    "id": null,
    "metadata": {},
    "name": "authorizeApproveMutation",
    "operationKind": "mutation",
    "text": "mutation authorizeApproveMutation(\n  $query: String!\n  $scopes: [String!]!\n) {\n  approveOAuthAuthorization(query: $query, scopes: $scopes)\n}\n"
  }
};
})();

(node as any).hash = "120d88560752ac41528bc0833e71bad1";

export default node;
