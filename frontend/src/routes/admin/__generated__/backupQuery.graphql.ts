/**
 * @generated SignedSource<<0109b75f673390f7cf9944f0ecf8ed31>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type backupQuery$variables = Record<PropertyKey, never>;
export type backupQuery$data = {
  readonly backup: {
    readonly latestRun: {
      readonly error: string | null | undefined;
      readonly fileName: string | null | undefined;
      readonly finishedAt: string;
      readonly manual: boolean;
      readonly size: number | null | undefined;
      readonly startedAt: string;
    } | null | undefined;
    readonly settings: {
      readonly generations: number;
      readonly intervalHours: number;
      readonly recipients: ReadonlyArray<string>;
      readonly updatedAt: string | null | undefined;
    };
  };
};
export type backupQuery = {
  response: backupQuery$data;
  variables: backupQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "BackupStatus",
    "kind": "LinkedField",
    "name": "backup",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "BackupSettings",
        "kind": "LinkedField",
        "name": "settings",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "recipients",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "intervalHours",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "generations",
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
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "BackupRun",
        "kind": "LinkedField",
        "name": "latestRun",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "manual",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "startedAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "finishedAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "fileName",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "size",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "error",
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
    "name": "backupQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "backupQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "bad348cdb6296a593458f13205de2238",
    "id": null,
    "metadata": {},
    "name": "backupQuery",
    "operationKind": "query",
    "text": "query backupQuery {\n  backup {\n    settings {\n      recipients\n      intervalHours\n      generations\n      updatedAt\n    }\n    latestRun {\n      manual\n      startedAt\n      finishedAt\n      fileName\n      size\n      error\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "74e892f7bec95cbad77930e905ec1a90";

export default node;
