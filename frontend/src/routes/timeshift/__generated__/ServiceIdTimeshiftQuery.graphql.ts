/**
 * @generated SignedSource<<c589743e335709297772a99dd02cea09>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type Role = "ADMIN" | "MEMBER" | "VIEWER" | "%future added value";
export type ServiceIdTimeshiftQuery$variables = {
  id: string;
};
export type ServiceIdTimeshiftQuery$data = {
  readonly node: {
    readonly __typename: "Service";
    readonly logoUrl: string | null | undefined;
    readonly name: string;
    readonly timeshiftPrograms: ReadonlyArray<{
      readonly endAt: string | null | undefined;
      readonly fromStart: boolean;
      readonly name: string;
      readonly promoting: boolean;
      readonly startAt: string;
    }>;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly viewer: {
    readonly role: Role;
  };
};
export type ServiceIdTimeshiftQuery = {
  response: ServiceIdTimeshiftQuery$data;
  variables: ServiceIdTimeshiftQuery$variables;
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
  "name": "name",
  "storageKey": null
},
v5 = {
  "kind": "InlineFragment",
  "selections": [
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
      "concreteType": "TimeshiftProgram",
      "kind": "LinkedField",
      "name": "timeshiftPrograms",
      "plural": true,
      "selections": [
        (v4/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "startAt",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "endAt",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "fromStart",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "promoting",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "type": "Service",
  "abstractKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ServiceIdTimeshiftQuery",
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
          (v5/*:: as any*/)
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
    "name": "ServiceIdTimeshiftQuery",
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
        "alias": null,
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
  "params": {
    "cacheID": "75799cd0d03feeffa46f46903d07a498",
    "id": null,
    "metadata": {},
    "name": "ServiceIdTimeshiftQuery",
    "operationKind": "query",
    "text": "query ServiceIdTimeshiftQuery(\n  $id: ID!\n) {\n  viewer {\n    role\n    id\n  }\n  node(id: $id) {\n    __typename\n    ... on Service {\n      name\n      logoUrl\n      timeshiftPrograms {\n        name\n        startAt\n        endAt\n        fromStart\n        promoting\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "e7cad71f3dc790f996be00a9bb391974";

export default node;
