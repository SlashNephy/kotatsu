/**
 * @generated SignedSource<<03cc576b6db1e1f1fa0f9d400395df70>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type accountBeginTotpMutation$variables = Record<PropertyKey, never>;
export type accountBeginTotpMutation$data = {
  readonly beginTotpEnrollment: {
    readonly secret: string;
    readonly uri: string;
  };
};
export type accountBeginTotpMutation = {
  response: accountBeginTotpMutation$data;
  variables: accountBeginTotpMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "TOTPEnrollment",
    "kind": "LinkedField",
    "name": "beginTotpEnrollment",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "secret",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "uri",
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
    "name": "accountBeginTotpMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "accountBeginTotpMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "ef93cf8e9aaf46183069a9723735f6ff",
    "id": null,
    "metadata": {},
    "name": "accountBeginTotpMutation",
    "operationKind": "mutation",
    "text": "mutation accountBeginTotpMutation {\n  beginTotpEnrollment {\n    secret\n    uri\n  }\n}\n"
  }
};
})();

(node as any).hash = "6e7ba13317f200139fd9a0ff945ad084";

export default node;
