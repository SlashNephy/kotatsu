/**
 * @generated SignedSource<<b56e63c0b717e57bfac95c642c7e6813>>
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
    "cacheID": "246ea2ac935d0c7e3eecdbe3cfc10fda",
    "id": null,
    "metadata": {},
    "name": "retentionQuery",
    "operationKind": "query",
    "text": "query retentionQuery {\n  systemSettings {\n    archiveRetentionDays\n    notificationRetentionDays\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "c7aaa39b10a726d953c5a80b8dd2ed49";

export default node;
