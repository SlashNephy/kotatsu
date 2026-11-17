/**
 * @generated SignedSource<<9ce4f1c18da100ca6ea9de30e954d7d2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaRecordDestination = "SIMKL" | "%future added value";
export type DramaRecordsQuery$variables = Record<PropertyKey, never>;
export type DramaRecordsQuery$data = {
  readonly dramaRecordSettings: {
    readonly destinations: ReadonlyArray<{
      readonly available: boolean;
      readonly connection: {
        readonly accountName: string;
        readonly connectedAt: string;
        readonly reauthorizationRequired: boolean;
        readonly watchlistError: string | null | undefined;
        readonly watchlistImportedAt: string | null | undefined;
      } | null | undefined;
      readonly destination: DramaRecordDestination;
    }>;
    readonly enabled: boolean;
    readonly thresholdPercent: number;
  };
};
export type DramaRecordsQuery = {
  response: DramaRecordsQuery$data;
  variables: DramaRecordsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "DramaRecordSettings",
    "kind": "LinkedField",
    "name": "dramaRecordSettings",
    "plural": false,
    "selections": [
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
        "name": "thresholdPercent",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "DramaRecordDestinationStatus",
        "kind": "LinkedField",
        "name": "destinations",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "destination",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "available",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "DramaRecordConnection",
            "kind": "LinkedField",
            "name": "connection",
            "plural": false,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "accountName",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "connectedAt",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "reauthorizationRequired",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "watchlistImportedAt",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "watchlistError",
                "storageKey": null
              }
            ],
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
    "name": "DramaRecordsQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "DramaRecordsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "a09f03f6b46229f09be06d3c6fdfcd62",
    "id": null,
    "metadata": {},
    "name": "DramaRecordsQuery",
    "operationKind": "query",
    "text": "query DramaRecordsQuery {\n  dramaRecordSettings {\n    enabled\n    thresholdPercent\n    destinations {\n      destination\n      available\n      connection {\n        accountName\n        connectedAt\n        reauthorizationRequired\n        watchlistImportedAt\n        watchlistError\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "b7d85e7987b8ed2960f7b1b164d5f816";

export default node;
