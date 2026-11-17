/**
 * @generated SignedSource<<dbf87b6308fd6538d69c1c4f04743c86>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsGoogleDriveBeginMutation$variables = Record<PropertyKey, never>;
export type integrationsGoogleDriveBeginMutation$data = {
  readonly beginGoogleDriveConnection: string;
};
export type integrationsGoogleDriveBeginMutation = {
  response: integrationsGoogleDriveBeginMutation$data;
  variables: integrationsGoogleDriveBeginMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "beginGoogleDriveConnection",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "integrationsGoogleDriveBeginMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "integrationsGoogleDriveBeginMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "201d5be598cd14554813fbeb71c74e6a",
    "id": null,
    "metadata": {},
    "name": "integrationsGoogleDriveBeginMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsGoogleDriveBeginMutation {\n  beginGoogleDriveConnection\n}\n"
  }
};
})();

(node as any).hash = "3f8cd98fcf411fbff0357bdea9aa6f40";

export default node;
