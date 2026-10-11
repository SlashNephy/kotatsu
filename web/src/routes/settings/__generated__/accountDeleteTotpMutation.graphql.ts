/**
 * @generated SignedSource<<32e3071523466198c0504b9b66eada83>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type accountDeleteTotpMutation$variables = Record<PropertyKey, never>;
export type accountDeleteTotpMutation$data = {
  readonly deleteTotp: {
    readonly totp: boolean;
  };
};
export type accountDeleteTotpMutation = {
  response: accountDeleteTotpMutation$data;
  variables: accountDeleteTotpMutation$variables;
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
    "name": "accountDeleteTotpMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "accountDeleteTotpMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "2ca489dac1980db328beca96bed6e34c",
    "id": null,
    "metadata": {},
    "name": "accountDeleteTotpMutation",
    "operationKind": "mutation",
    "text": "mutation accountDeleteTotpMutation {\n  deleteTotp {\n    totp\n  }\n}\n"
  }
};
})();

(node as any).hash = "bc65537e06a116c9d1a6d82b325cf776";

export default node;
