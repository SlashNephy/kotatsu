/**
 * @generated SignedSource<<dee4aac575369480f2790970f22d255a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsGoogleDriveDisconnectMutation$variables = Record<PropertyKey, never>;
export type integrationsGoogleDriveDisconnectMutation$data = {
  readonly disconnectGoogleDrive: {
    readonly configured: boolean;
  };
};
export type integrationsGoogleDriveDisconnectMutation = {
  response: integrationsGoogleDriveDisconnectMutation$data;
  variables: integrationsGoogleDriveDisconnectMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "GoogleIntegration",
    "kind": "LinkedField",
    "name": "disconnectGoogleDrive",
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
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "integrationsGoogleDriveDisconnectMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "integrationsGoogleDriveDisconnectMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "8c6c863dd57fc89d2930e5a90272a84b",
    "id": null,
    "metadata": {},
    "name": "integrationsGoogleDriveDisconnectMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsGoogleDriveDisconnectMutation {\n  disconnectGoogleDrive {\n    configured\n  }\n}\n"
  }
};
})();

(node as any).hash = "94831736f5802c2e7df42931c9533fe1";

export default node;
