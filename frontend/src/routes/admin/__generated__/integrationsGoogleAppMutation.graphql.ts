/**
 * @generated SignedSource<<9fc0ea1124602aae1211bde8987e18ba>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type GoogleAppInput = {
  clientId: string;
  clientSecret?: string | null | undefined;
};
export type integrationsGoogleAppMutation$variables = {
  input: GoogleAppInput;
};
export type integrationsGoogleAppMutation$data = {
  readonly updateGoogleApp: {
    readonly configured: boolean;
  };
};
export type integrationsGoogleAppMutation = {
  response: integrationsGoogleAppMutation$data;
  variables: integrationsGoogleAppMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "GoogleIntegration",
    "kind": "LinkedField",
    "name": "updateGoogleApp",
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
    "name": "integrationsGoogleAppMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsGoogleAppMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "fd91af71dc2ca661d17e69b1eaefc40c",
    "id": null,
    "metadata": {},
    "name": "integrationsGoogleAppMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsGoogleAppMutation(\n  $input: GoogleAppInput!\n) {\n  updateGoogleApp(input: $input) {\n    configured\n  }\n}\n"
  }
};
})();

(node as any).hash = "641ea5cc25d66ca283a324d75622c057";

export default node;
