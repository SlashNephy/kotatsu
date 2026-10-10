/**
 * @generated SignedSource<<81bbed718d35cece215883c2047cdaf1>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ServiceIdLiveQuery$variables = {
  from: string;
  id: string;
  to: string;
};
export type ServiceIdLiveQuery$data = {
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly name: string;
    }>;
  }>;
  readonly node: {
    readonly __typename: "Service";
    readonly name: string;
    readonly streamUrl: string;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
};
export type ServiceIdLiveQuery = {
  response: ServiceIdLiveQuery$data;
  variables: ServiceIdLiveQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "from"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "id"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "to"
},
v3 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
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
  "kind": "InlineFragment",
  "selections": [
    (v5/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "streamUrl",
      "storageKey": null
    }
  ],
  "type": "Service",
  "abstractKey": null
},
v7 = [
  {
    "kind": "Variable",
    "name": "from",
    "variableName": "from"
  },
  {
    "items": [
      {
        "kind": "Variable",
        "name": "services.0",
        "variableName": "id"
      }
    ],
    "kind": "ListValue",
    "name": "services"
  },
  {
    "kind": "Variable",
    "name": "to",
    "variableName": "to"
  }
],
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "ServiceIdLiveQuery",
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v4/*:: as any*/),
          (v6/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v7/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "programs",
            "plural": true,
            "selections": [
              (v5/*:: as any*/)
            ],
            "storageKey": null
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
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "ServiceIdLiveQuery",
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v4/*:: as any*/),
          (v6/*:: as any*/),
          (v8/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v7/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "programs",
            "plural": true,
            "selections": [
              (v5/*:: as any*/),
              (v8/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "b087981af015296f5b62eb0a3708080d",
    "id": null,
    "metadata": {},
    "name": "ServiceIdLiveQuery",
    "operationKind": "query",
    "text": "query ServiceIdLiveQuery(\n  $id: ID!\n  $from: Time!\n  $to: Time!\n) {\n  node(id: $id) {\n    __typename\n    ... on Service {\n      name\n      streamUrl\n    }\n    id\n  }\n  guide(from: $from, to: $to, services: [$id]) {\n    programs {\n      name\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "76c55712f68095056820f8bcafae91b2";

export default node;
