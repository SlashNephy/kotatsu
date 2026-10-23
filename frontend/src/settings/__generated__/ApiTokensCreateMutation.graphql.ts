/**
 * @generated SignedSource<<f2d3054bc161b96ab150c76fe674921f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ApiTokensCreateMutation$variables = {
  name: string;
};
export type ApiTokensCreateMutation$data = {
  readonly createApiToken: {
    readonly token: string;
  };
};
export type ApiTokensCreateMutation = {
  response: ApiTokensCreateMutation$data;
  variables: ApiTokensCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "name"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "name",
        "variableName": "name"
      }
    ],
    "concreteType": "CreatedApiToken",
    "kind": "LinkedField",
    "name": "createApiToken",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "token",
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
    "name": "ApiTokensCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ApiTokensCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "56fa98ee9fd0f78f4e02bf2b9c936214",
    "id": null,
    "metadata": {},
    "name": "ApiTokensCreateMutation",
    "operationKind": "mutation",
    "text": "mutation ApiTokensCreateMutation(\n  $name: String!\n) {\n  createApiToken(name: $name) {\n    token\n  }\n}\n"
  }
};
})();

(node as any).hash = "5a56f09c421cdf66fe17e33b8987e5cf";

export default node;
