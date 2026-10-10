/**
 * @generated SignedSource<<9321d221fe66b85de3f11e7dfd585a6a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type ProgramDetailQuery$variables = {
  id: string;
};
export type ProgramDetailQuery$data = {
  readonly node: {
    readonly __typename: "Program";
    readonly description: string;
    readonly endAt: string;
    readonly endUndecided: boolean;
    readonly extended: ReadonlyArray<{
      readonly heading: string;
      readonly text: string;
    }>;
    readonly genres: ReadonlyArray<{
      readonly lv1: number;
    }>;
    readonly id: string;
    readonly isFree: boolean;
    readonly name: string;
    readonly service: {
      readonly name: string;
    };
    readonly startAt: string;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly viewer: {
    readonly role: Role;
  };
};
export type ProgramDetailQuery = {
  response: ProgramDetailQuery$data;
  variables: ProgramDetailQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
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
    "variableName": "id"
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
  "name": "id",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "description",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endUndecided",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "isFree",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "concreteType": "Genre",
  "kind": "LinkedField",
  "name": "genres",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "lv1",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "concreteType": "ExtendedItem",
  "kind": "LinkedField",
  "name": "extended",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "heading",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "text",
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
    "name": "ProgramDetailQuery",
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
        "alias": null,
        "args": (v2/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v3/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v4/*:: as any*/),
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v5/*:: as any*/)
                ],
                "storageKey": null
              },
              (v11/*:: as any*/),
              (v12/*:: as any*/)
            ],
            "type": "Program",
            "abstractKey": null
          }
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ProgramDetailQuery",
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
          (v4/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v2/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v5/*:: as any*/),
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "Service",
                "kind": "LinkedField",
                "name": "service",
                "plural": false,
                "selections": [
                  (v5/*:: as any*/),
                  (v4/*:: as any*/)
                ],
                "storageKey": null
              },
              (v11/*:: as any*/),
              (v12/*:: as any*/)
            ],
            "type": "Program",
            "abstractKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "e6836d3449e1f7df82284129bc3b97b9",
    "id": null,
    "metadata": {},
    "name": "ProgramDetailQuery",
    "operationKind": "query",
    "text": "query ProgramDetailQuery(\n  $id: ID!\n) {\n  viewer {\n    role\n    id\n  }\n  node(id: $id) {\n    __typename\n    ... on Program {\n      id\n      name\n      description\n      startAt\n      endAt\n      endUndecided\n      isFree\n      service {\n        name\n        id\n      }\n      genres {\n        lv1\n      }\n      extended {\n        heading\n        text\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "6633c1894af35c0c078551e13e194e3d";

export default node;
