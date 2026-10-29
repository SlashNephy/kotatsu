/**
 * @generated SignedSource<<fe9dc4c3342f01cea571ce7adc7d3e82>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
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
      readonly name: string;
      readonly playableFrom: string;
      readonly startAt: string;
    }>;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
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
v1 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v4 = {
  "kind": "InlineFragment",
  "selections": [
    (v3/*:: as any*/),
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
        (v3/*:: as any*/),
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
          "name": "playableFrom",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "type": "Service",
  "abstractKey": null
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
        "args": (v1/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v4/*:: as any*/)
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
        "args": (v1/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v4/*:: as any*/),
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
    ]
  },
  "params": {
    "cacheID": "53e59a70fd45fc007358cf70691c604e",
    "id": null,
    "metadata": {},
    "name": "ServiceIdTimeshiftQuery",
    "operationKind": "query",
    "text": "query ServiceIdTimeshiftQuery(\n  $id: ID!\n) {\n  node(id: $id) {\n    __typename\n    ... on Service {\n      name\n      logoUrl\n      timeshiftPrograms {\n        name\n        startAt\n        endAt\n        playableFrom\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "53053810a05cf3dee4a6fca498b19beb";

export default node;
