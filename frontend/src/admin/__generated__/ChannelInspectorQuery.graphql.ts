/**
 * @generated SignedSource<<cb1a6b0abc7dfa71c9fc6234a211a2e2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ChannelInspectorQuery$variables = Record<PropertyKey, never>;
export type ChannelInspectorQuery$data = {
  readonly services: ReadonlyArray<{
    readonly channel: string;
    readonly channelType: {
      readonly displayName: string;
      readonly type: string;
    };
    readonly id: string;
  }>;
};
export type ChannelInspectorQuery = {
  response: ChannelInspectorQuery$data;
  variables: ChannelInspectorQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Literal",
        "name": "includeHidden",
        "value": true
      }
    ],
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "services",
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
        "name": "channel",
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
      }
    ],
    "storageKey": "services(includeHidden:true)"
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "ChannelInspectorQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "ChannelInspectorQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "143b9cac829189ad617ee2f18b0d6f6b",
    "id": null,
    "metadata": {},
    "name": "ChannelInspectorQuery",
    "operationKind": "query",
    "text": "query ChannelInspectorQuery {\n  services(includeHidden: true) {\n    id\n    channel\n    channelType {\n      type\n      displayName\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "90d8a9d5a7a70314a6e1230e7d6a3e1d";

export default node;
