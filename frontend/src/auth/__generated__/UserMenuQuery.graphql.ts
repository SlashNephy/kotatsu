/**
 * @generated SignedSource<<a6be0cdfd7c1f8c4430cd5e0f28ac853>>
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
    readonly avatarUrl: string | null | undefined;
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
  "name": "avatarUrl",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "version",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "concreteType": "AppVersion",
  "kind": "LinkedField",
  "name": "version",
  "plural": false,
  "selections": [
    (v3/*:: as any*/),
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
        (v3/*:: as any*/),
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
          (v1/*:: as any*/),
          (v2/*:: as any*/)
        ],
        "storageKey": null
      },
      (v4/*:: as any*/)
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
          (v2/*:: as any*/),
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
      (v4/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "b6ffca7b55c64f34e6817a855ab0e5d4",
    "id": null,
    "metadata": {},
    "name": "UserMenuQuery",
    "operationKind": "query",
    "text": "query UserMenuQuery {\n  viewer {\n    displayName\n    username\n    avatarUrl\n    id\n  }\n  version {\n    version\n    revision\n    modified\n    latestRelease {\n      version\n      url\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "380c48b161413b64f772537eff6d03aa";

export default node;
