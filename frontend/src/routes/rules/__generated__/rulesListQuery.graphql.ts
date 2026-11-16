/**
 * @generated SignedSource<<38583d6bd9bf6caadd41140d55a9ba21>>
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
  readonly ruleGroups: ReadonlyArray<{
    readonly id: string;
    readonly name: string;
  }>;
  readonly rules: ReadonlyArray<{
    readonly conditions: {
      readonly channelTypes: ReadonlyArray<string>;
      readonly keywords: ReadonlyArray<{
        readonly text: string;
      }>;
    };
    readonly createdAt: string;
    readonly enabled: boolean;
    readonly group: {
      readonly id: string;
    } | null | undefined;
    readonly id: string;
    readonly memo: string;
    readonly name: string;
    readonly upcomingReservationCount: number;
    readonly updatedAt: string;
    readonly validFrom: string | null | undefined;
    readonly validUntil: string | null | undefined;
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
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "concreteType": "RuleGroup",
  "kind": "LinkedField",
  "name": "ruleGroups",
  "plural": true,
  "selections": [
    (v2/*:: as any*/),
    (v3/*:: as any*/)
  ],
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "concreteType": "Rule",
  "kind": "LinkedField",
  "name": "rules",
  "plural": true,
  "selections": [
    (v2/*:: as any*/),
    (v3/*:: as any*/),
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
      "name": "validFrom",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "validUntil",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "createdAt",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "updatedAt",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "upcomingReservationCount",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "RuleGroup",
      "kind": "LinkedField",
      "name": "group",
      "plural": false,
      "selections": [
        (v2/*:: as any*/)
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
      (v4/*:: as any*/),
      (v5/*:: as any*/)
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
      (v4/*:: as any*/),
      (v5/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "51b7bdc4dd0b109288613b8eb60fb4a8",
    "id": null,
    "metadata": {},
    "name": "rulesListQuery",
    "operationKind": "query",
    "text": "query rulesListQuery {\n  viewer {\n    role\n    id\n  }\n  channelTypes {\n    type\n    displayName\n  }\n  ruleGroups {\n    id\n    name\n  }\n  rules {\n    id\n    name\n    enabled\n    memo\n    conditions {\n      keywords {\n        text\n      }\n      channelTypes\n    }\n    validFrom\n    validUntil\n    createdAt\n    updatedAt\n    upcomingReservationCount\n    group {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "8def8f0da3900e21c9f341a044b8b9a9";

export default node;
