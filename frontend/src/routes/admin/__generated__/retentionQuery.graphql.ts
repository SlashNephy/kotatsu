/**
 * @generated SignedSource<<718a8dbc57e587b2899187ccc5154583>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type retentionQuery$variables = Record<PropertyKey, never>;
export type retentionQuery$data = {
  readonly systemSettings: {
    readonly archiveRetentionDays: number;
    readonly notificationRetentionDays: number;
    readonly updatedAt: string | null | undefined;
    readonly watchProgressRetentionDays: number;
  };
};
export type retentionQuery = {
  response: retentionQuery$data;
  variables: retentionQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemSettings",
    "kind": "LinkedField",
    "name": "systemSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "archiveRetentionDays",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "notificationRetentionDays",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "watchProgressRetentionDays",
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
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "retentionQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "retentionQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "4ebe23fd8e9e0f8063dcd4d35cc393f6",
    "id": null,
    "metadata": {},
    "name": "retentionQuery",
    "operationKind": "query",
    "text": "query retentionQuery {\n  systemSettings {\n    archiveRetentionDays\n    notificationRetentionDays\n    watchProgressRetentionDays\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "3dc68dc8777ad86911f10a833464db9f";

export default node;
