/**
 * @generated SignedSource<<1d7a7ce4c282bf1942a0a82146e4467e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type securityRegenerateMutation$variables = Record<PropertyKey, never>;
export type securityRegenerateMutation$data = {
  readonly regenerateRecoveryCodes: {
    readonly codes: ReadonlyArray<string>;
  };
};
export type securityRegenerateMutation = {
  response: securityRegenerateMutation$data;
  variables: securityRegenerateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "RecoveryCodes",
    "kind": "LinkedField",
    "name": "regenerateRecoveryCodes",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "codes",
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
    "name": "securityRegenerateMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "securityRegenerateMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "b309b13d2a1ef0e2174b64b2e53796a0",
    "id": null,
    "metadata": {},
    "name": "securityRegenerateMutation",
    "operationKind": "mutation",
    "text": "mutation securityRegenerateMutation {\n  regenerateRecoveryCodes {\n    codes\n  }\n}\n"
  }
};
})();

(node as any).hash = "292caac907f7e7ab3a3a35bc514cb3af";

export default node;
