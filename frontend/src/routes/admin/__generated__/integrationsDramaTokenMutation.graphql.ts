/**
 * @generated SignedSource<<37d2793e806f6776f95f28da0ff85d05>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsDramaTokenMutation$variables = {
  token: string;
};
export type integrationsDramaTokenMutation$data = {
  readonly updateDramaLinkingToken: {
    readonly tokenConfigured: boolean;
    readonly tokenHint: string | null | undefined;
  };
};
export type integrationsDramaTokenMutation = {
  response: integrationsDramaTokenMutation$data;
  variables: integrationsDramaTokenMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "token"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "token",
        "variableName": "token"
      }
    ],
    "concreteType": "DramaLinkingStatus",
    "kind": "LinkedField",
    "name": "updateDramaLinkingToken",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "tokenConfigured",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "tokenHint",
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
    "name": "integrationsDramaTokenMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsDramaTokenMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "467037540db468d99fab3e152931b8ef",
    "id": null,
    "metadata": {},
    "name": "integrationsDramaTokenMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsDramaTokenMutation(\n  $token: String!\n) {\n  updateDramaLinkingToken(token: $token) {\n    tokenConfigured\n    tokenHint\n  }\n}\n"
  }
};
})();

(node as any).hash = "c440373e315acb95b6fbef318a3d0edc";

export default node;
