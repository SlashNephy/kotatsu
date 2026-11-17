/**
 * @generated SignedSource<<401a9a09a10409a87bb8efe7bd5ba7c7>>
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
    readonly auditClientInfoRetentionDays: number;
    readonly auditRetentionDays: number;
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
        "name": "auditRetentionDays",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "auditClientInfoRetentionDays",
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
    "cacheID": "896722de6e13caaa5a81cdab48fbd56e",
    "id": null,
    "metadata": {},
    "name": "retentionQuery",
    "operationKind": "query",
    "text": "query retentionQuery {\n  systemSettings {\n    archiveRetentionDays\n    notificationRetentionDays\n    watchProgressRetentionDays\n    networkActivityRetentionDays\n    auditRetentionDays\n    auditClientInfoRetentionDays\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "2a9c9d701f809197076f60343fc998ae";

export default node;
