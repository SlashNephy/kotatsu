/**
 * @generated SignedSource<<2c7b3448f2414c7abbad13a80cb60db6>>
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
    readonly networkActivityRetentionDays: number;
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
        "name": "networkActivityRetentionDays",
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
    "cacheID": "1910bdbec0574d2215fe27b71b1a70de",
    "id": null,
    "metadata": {},
    "name": "retentionQuery",
    "operationKind": "query",
    "text": "query retentionQuery {\n  systemSettings {\n    archiveRetentionDays\n    notificationRetentionDays\n    watchProgressRetentionDays\n    networkActivityRetentionDays\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "73653ff84b65eb55285026abd75b1794";

export default node;
