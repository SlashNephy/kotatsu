/**
 * @generated SignedSource<<4c150c930d2d27237e5dccf01d5a8fb9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type securityBeginTotpMutation$variables = Record<PropertyKey, never>;
export type securityBeginTotpMutation$data = {
  readonly beginTotpEnrollment: {
    readonly secret: string;
    readonly uri: string;
  };
};
export type securityBeginTotpMutation = {
  response: securityBeginTotpMutation$data;
  variables: securityBeginTotpMutation$variables;
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
    "name": "securityBeginTotpMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "securityBeginTotpMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "e29d539884eb5fc5a5602357c6a705ed",
    "id": null,
    "metadata": {},
    "name": "securityBeginTotpMutation",
    "operationKind": "mutation",
    "text": "mutation securityBeginTotpMutation {\n  beginTotpEnrollment {\n    secret\n    uri\n  }\n}\n"
  }
};
})();

(node as any).hash = "976f82e3a847b1c2d0831784c09caf0d";

export default node;
