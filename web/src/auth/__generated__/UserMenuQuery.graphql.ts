/**
 * @generated SignedSource<<0816460ae2f5405c0bd455425669c211>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type UserMenuQuery$variables = Record<PropertyKey, never>;
export type UserMenuQuery$data = {
  readonly version: {
    readonly latestRelease: {
      readonly url: string;
      readonly version: string;
    } | null | undefined;
    readonly modified: boolean;
    readonly revision: string | null | undefined;
    readonly version: string;
  };
  readonly viewer: {
    readonly displayName: string;
    readonly username: string;
  };
};
export type UserMenuQuery = {
  response: UserMenuQuery$data;
  variables: UserMenuQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "username",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "version",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "concreteType": "AppVersion",
  "kind": "LinkedField",
  "name": "version",
  "plural": false,
  "selections": [
    (v2/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "revision",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "modified",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "Release",
      "kind": "LinkedField",
      "name": "latestRelease",
      "plural": false,
      "selections": [
        (v2/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "url",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "UserMenuQuery",
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
      (v3/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "UserMenuQuery",
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
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "id",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v3/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "5ad73480f2e438b98e335b46fc98ee1c",
    "id": null,
    "metadata": {},
    "name": "UserMenuQuery",
    "operationKind": "query",
    "text": "query UserMenuQuery {\n  viewer {\n    displayName\n    username\n    id\n  }\n  version {\n    version\n    revision\n    modified\n    latestRelease {\n      version\n      url\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "e21d5be91cd61877ed1fd0192759233c";

export default node;
