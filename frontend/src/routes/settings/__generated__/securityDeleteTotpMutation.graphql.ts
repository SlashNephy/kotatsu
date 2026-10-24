/**
 * @generated SignedSource<<4c32f75f59949c955128f5c059ef8fe5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type securityDeleteTotpMutation$variables = Record<PropertyKey, never>;
export type securityDeleteTotpMutation$data = {
  readonly deleteTotp: {
    readonly totp: boolean;
  };
};
export type securityDeleteTotpMutation = {
  response: securityDeleteTotpMutation$data;
  variables: securityDeleteTotpMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "SecondFactorStatus",
    "kind": "LinkedField",
    "name": "deleteTotp",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "totp",
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
    "name": "securityDeleteTotpMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "securityDeleteTotpMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "ca3c34b91b6ead1f1016e16116a7ba12",
    "id": null,
    "metadata": {},
    "name": "securityDeleteTotpMutation",
    "operationKind": "mutation",
    "text": "mutation securityDeleteTotpMutation {\n  deleteTotp {\n    totp\n  }\n}\n"
  }
};
})();

(node as any).hash = "6a13d1426509b0c3a996c752c3061fe1";

export default node;
