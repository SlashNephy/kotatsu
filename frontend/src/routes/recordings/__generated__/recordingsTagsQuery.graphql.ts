/**
 * @generated SignedSource<<c087eee9c454bd6fd57696f5726fd282>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type recordingsTagsQuery$variables = {
  ruleId: string;
  withRule: boolean;
};
export type recordingsTagsQuery$data = {
  readonly ruleFilter?: {
    readonly __typename: "Rule";
    readonly name: string;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly displayName: string;
    };
    readonly id: string;
    readonly logoUrl: string | null | undefined;
    readonly name: string;
  }>;
  readonly tags: ReadonlyArray<{
    readonly id: string;
    readonly name: string;
    readonly recordings: number;
  }>;
  readonly viewer: {
    readonly role: Role;
  };
};
export type recordingsTagsQuery = {
  response: recordingsTagsQuery$data;
  variables: recordingsTagsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "ruleId"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "withRule"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "role",
  "storageKey": null
},
v2 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "ruleId"
  }
],
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v5 = {
  "kind": "InlineFragment",
  "selections": [
    (v4/*:: as any*/)
  ],
  "type": "Rule",
  "abstractKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "concreteType": "Tag",
  "kind": "LinkedField",
  "name": "tags",
  "plural": true,
  "selections": [
    (v6/*:: as any*/),
    (v4/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "recordings",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "services",
  "plural": true,
  "selections": [
    (v6/*:: as any*/),
    (v4/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "logoUrl",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "ChannelType",
      "kind": "LinkedField",
      "name": "channelType",
      "plural": false,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "displayName",
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
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "recordingsTagsQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v1/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "condition": "withRule",
        "kind": "Condition",
        "passingValue": true,
        "selections": [
          {
            "alias": "ruleFilter",
            "args": (v2/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v3/*:: as any*/),
              (v5/*:: as any*/)
            ],
            "storageKey": null
          }
        ]
      },
      (v7/*:: as any*/),
      (v8/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "recordingsTagsQuery",
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "User",
        "kind": "LinkedField",
        "name": "viewer",
        "plural": false,
        "selections": [
          (v1/*:: as any*/),
          (v6/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "condition": "withRule",
        "kind": "Condition",
        "passingValue": true,
        "selections": [
          {
            "alias": "ruleFilter",
            "args": (v2/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v3/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/)
            ],
            "storageKey": null
          }
        ]
      },
      (v7/*:: as any*/),
      (v8/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "376a85150ac3bd3d8d678a121deafb92",
    "id": null,
    "metadata": {},
    "name": "recordingsTagsQuery",
    "operationKind": "query",
    "text": "query recordingsTagsQuery(\n  $ruleId: ID!\n  $withRule: Boolean!\n) {\n  viewer {\n    role\n    id\n  }\n  ruleFilter: node(id: $ruleId) @include(if: $withRule) {\n    __typename\n    ... on Rule {\n      name\n    }\n    id\n  }\n  tags {\n    id\n    name\n    recordings\n  }\n  services {\n    id\n    name\n    logoUrl\n    channelType {\n      displayName\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "f37a1956d6ee9803e233b8815773b3b9";

export default node;
