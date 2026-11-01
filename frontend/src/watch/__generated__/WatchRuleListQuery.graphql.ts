/**
 * @generated SignedSource<<e23bf027ae45c41b9ab4663153608b1c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type WatchRuleListQuery$variables = Record<PropertyKey, never>;
export type WatchRuleListQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly type: string;
  }>;
  readonly watchRules: ReadonlyArray<{
    readonly conditions: {
      readonly channelTypes: ReadonlyArray<string>;
      readonly keywords: ReadonlyArray<{
        readonly text: string;
      }>;
    };
    readonly enabled: boolean;
    readonly id: string;
    readonly name: string;
  }>;
};
export type WatchRuleListQuery = {
  response: WatchRuleListQuery$data;
  variables: WatchRuleListQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
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
  {
    "alias": null,
    "args": null,
    "concreteType": "WatchRule",
    "kind": "LinkedField",
    "name": "watchRules",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
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
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "WatchRuleListQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "WatchRuleListQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "93f32545950a1cf40b97b22465c0a75d",
    "id": null,
    "metadata": {},
    "name": "WatchRuleListQuery",
    "operationKind": "query",
    "text": "query WatchRuleListQuery {\n  channelTypes {\n    type\n    displayName\n  }\n  watchRules {\n    id\n    name\n    enabled\n    conditions {\n      keywords {\n        text\n      }\n      channelTypes\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "f8528e896f395566eaaf1a6afa73e5c8";

export default node;
