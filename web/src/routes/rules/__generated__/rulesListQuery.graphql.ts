/**
 * @generated SignedSource<<01eb85a2f67723e38005e519a2174711>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type rulesListQuery$variables = Record<PropertyKey, never>;
export type rulesListQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly type: string;
  }>;
  readonly rules: ReadonlyArray<{
    readonly conditions: {
      readonly channelTypes: ReadonlyArray<string>;
      readonly keywords: ReadonlyArray<{
        readonly text: string;
      }>;
    };
    readonly enabled: boolean;
    readonly id: string;
    readonly memo: string;
    readonly name: string;
    readonly updatedAt: string;
  }>;
  readonly viewer: {
    readonly role: Role;
  };
};
export type rulesListQuery = {
  response: rulesListQuery$data;
  variables: rulesListQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "concreteType": "ChannelType",
  "kind": "LinkedField",
  "name": "channelTypes",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "type",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "displayName",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "concreteType": "Rule",
  "kind": "LinkedField",
  "name": "rules",
  "plural": true,
  "selections": [
    (v2/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "name",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "enabled",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "memo",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "ProgramConditions",
      "kind": "LinkedField",
      "name": "conditions",
      "plural": false,
      "selections": [
        {
          "alias": null,
          "args": null,
          "concreteType": "Keyword",
          "kind": "LinkedField",
          "name": "keywords",
          "plural": true,
          "selections": [
            {
              "alias": null,
              "args": null,
              "kind": "ScalarField",
              "name": "text",
              "storageKey": null
            }
          ],
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "channelTypes",
          "storageKey": null
        }
      ],
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "updatedAt",
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
    "name": "rulesListQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v0/*:: as any*/)
        ],
        "storageKey": null
      },
      (v1/*:: as any*/),
      (v3/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "rulesListQuery",
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
          (v2/*:: as any*/)
        ],
        "storageKey": null
      },
      (v1/*:: as any*/),
      (v3/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "69c2b3ed476bc5d051abf2e01d3f4915",
    "id": null,
    "metadata": {},
    "name": "rulesListQuery",
    "operationKind": "query",
    "text": "query rulesListQuery {\n  viewer {\n    role\n    id\n  }\n  channelTypes {\n    type\n    displayName\n  }\n  rules {\n    id\n    name\n    enabled\n    memo\n    conditions {\n      keywords {\n        text\n      }\n      channelTypes\n    }\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "889bb2d9bf4e16d7bde6985d196fcf89";

export default node;
