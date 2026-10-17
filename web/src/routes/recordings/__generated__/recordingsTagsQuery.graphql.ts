/**
 * @generated SignedSource<<a61f93aa8c7a2d2e3a14046a76057a33>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
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
    readonly id: string;
    readonly name: string;
  }>;
  readonly tags: ReadonlyArray<{
    readonly id: string;
    readonly name: string;
    readonly recordings: number;
  }>;
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
v1 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "ruleId"
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
    (v3/*:: as any*/)
  ],
  "type": "Rule",
  "abstractKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "Tag",
  "kind": "LinkedField",
  "name": "tags",
  "plural": true,
  "selections": [
    (v5/*:: as any*/),
    (v3/*:: as any*/),
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
v7 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "services",
  "plural": true,
  "selections": [
    (v5/*:: as any*/),
    (v3/*:: as any*/)
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
        "condition": "withRule",
        "kind": "Condition",
        "passingValue": true,
        "selections": [
          {
            "alias": "ruleFilter",
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
        ]
      },
      (v6/*:: as any*/),
      (v7/*:: as any*/)
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
        "condition": "withRule",
        "kind": "Condition",
        "passingValue": true,
        "selections": [
          {
            "alias": "ruleFilter",
            "args": (v1/*:: as any*/),
            "concreteType": null,
            "kind": "LinkedField",
            "name": "node",
            "plural": false,
            "selections": [
              (v2/*:: as any*/),
              (v4/*:: as any*/),
              (v5/*:: as any*/)
            ],
            "storageKey": null
          }
        ]
      },
      (v6/*:: as any*/),
      (v7/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "040ad15c3fb393eba222f56cf86af0ea",
    "id": null,
    "metadata": {},
    "name": "recordingsTagsQuery",
    "operationKind": "query",
    "text": "query recordingsTagsQuery(\n  $ruleId: ID!\n  $withRule: Boolean!\n) {\n  ruleFilter: node(id: $ruleId) @include(if: $withRule) {\n    __typename\n    ... on Rule {\n      name\n    }\n    id\n  }\n  tags {\n    id\n    name\n    recordings\n  }\n  services {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "7b7c8404a3e1c26bd5962d305fe2f391";

export default node;
