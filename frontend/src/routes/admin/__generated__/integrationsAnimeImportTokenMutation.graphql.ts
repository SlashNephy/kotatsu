/**
 * @generated SignedSource<<32489b77679c9ccae395b8d75cc9accf>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsAnimeImportTokenMutation$variables = {
  annictToken: string;
};
export type integrationsAnimeImportTokenMutation$data = {
  readonly updateAnimeImportToken: {
    readonly configured: boolean;
  };
};
export type integrationsAnimeImportTokenMutation = {
  response: integrationsAnimeImportTokenMutation$data;
  variables: integrationsAnimeImportTokenMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "annictToken"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "annictToken",
        "variableName": "annictToken"
      }
    ],
    "concreteType": "AnimeImportToken",
    "kind": "LinkedField",
    "name": "updateAnimeImportToken",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "configured",
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
    "name": "integrationsAnimeImportTokenMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsAnimeImportTokenMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "56d442db77b0a620e0b0165fdcbaeff0",
    "id": null,
    "metadata": {},
    "name": "integrationsAnimeImportTokenMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsAnimeImportTokenMutation(\n  $annictToken: String!\n) {\n  updateAnimeImportToken(annictToken: $annictToken) {\n    configured\n  }\n}\n"
  }
};
})();

(node as any).hash = "12bfb2aa7630c4ac43aa4eb472c840ec";

export default node;
