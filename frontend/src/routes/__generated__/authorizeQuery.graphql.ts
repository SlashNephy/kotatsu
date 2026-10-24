/**
 * @generated SignedSource<<f33d6e2aea4236b925820f99c5a73645>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type authorizeQuery$variables = {
  query: string;
};
export type authorizeQuery$data = {
  readonly oauthAuthorization: {
    readonly authorization: {
      readonly allowedScopes: ReadonlyArray<string>;
      readonly clientId: string;
      readonly clientName: string;
      readonly clientUri: string | null | undefined;
      readonly redirectUri: string;
      readonly scopes: ReadonlyArray<string>;
    } | null | undefined;
    readonly errorRedirectUrl: string | null | undefined;
  };
  readonly passkeys: ReadonlyArray<{
    readonly id: string;
  }>;
  readonly secondFactors: {
    readonly remainingRecoveryCodes: number;
    readonly totp: boolean;
  };
};
export type authorizeQuery = {
  response: authorizeQuery$data;
  variables: authorizeQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "query"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "query",
        "variableName": "query"
      }
    ],
    "concreteType": "OAuthAuthorizationCheck",
    "kind": "LinkedField",
    "name": "oauthAuthorization",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "OAuthAuthorization",
        "kind": "LinkedField",
        "name": "authorization",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "clientId",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "clientName",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "clientUri",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "redirectUri",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "scopes",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "allowedScopes",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "errorRedirectUrl",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
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
  {
    "alias": null,
    "args": null,
    "concreteType": "Passkey",
    "kind": "LinkedField",
    "name": "passkeys",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
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
    "name": "authorizeQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "authorizeQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "b094b539922b09ee72c90290d052a4b1",
    "id": null,
    "metadata": {},
    "name": "authorizeQuery",
    "operationKind": "query",
    "text": "query authorizeQuery(\n  $query: String!\n) {\n  oauthAuthorization(query: $query) {\n    authorization {\n      clientId\n      clientName\n      clientUri\n      redirectUri\n      scopes\n      allowedScopes\n    }\n    errorRedirectUrl\n  }\n  secondFactors {\n    totp\n    remainingRecoveryCodes\n  }\n  passkeys {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "675df1a978f43e8a182920f878ac5d34";

export default node;
