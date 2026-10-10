/**
 * @generated SignedSource<<81b1fbcc59101e419869915ac18fcd49>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type passwordViewerQuery$variables = Record<PropertyKey, never>;
export type passwordViewerQuery$data = {
  readonly passkeys: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
  readonly viewer: {
    readonly displayName: string;
    readonly username: string;
  };
};
export type passwordViewerQuery = {
  response: passwordViewerQuery$data;
  variables: passwordViewerQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "username",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "concreteType": "SecondFactorStatus",
  "kind": "LinkedField",
  "name": "secondFactors",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "totp",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "remainingRecoveryCodes",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "concreteType": "Passkey",
  "kind": "LinkedField",
  "name": "passkeys",
  "plural": true,
  "selections": [
    (v3/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "passwordViewerQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/),
          (v1/*:: as any*/)
        ],
        "storageKey": null
      },
      (v2/*:: as any*/),
      (v4/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "passwordViewerQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/),
          (v1/*:: as any*/),
          (v3/*:: as any*/)
        ],
        "storageKey": null
      },
      (v2/*:: as any*/),
      (v4/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "1a2fd8c771c6eb5f7afbaec8a68dfa18",
    "id": null,
    "metadata": {},
    "name": "passwordViewerQuery",
    "operationKind": "query",
    "text": "query passwordViewerQuery {\n  viewer {\n    username\n    displayName\n    id\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "1b078fb24374ce48302290f34cfa9e97";

export default node;
