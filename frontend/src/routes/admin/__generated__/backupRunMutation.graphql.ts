/**
 * @generated SignedSource<<090545e41bd654f810a800d7121fec02>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type backupRunMutation$variables = Record<PropertyKey, never>;
export type backupRunMutation$data = {
  readonly runBackup: {
    readonly settings: {
      readonly updatedAt: string | null | undefined;
    };
  };
};
export type backupRunMutation = {
  response: backupRunMutation$data;
  variables: backupRunMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "BackupStatus",
    "kind": "LinkedField",
    "name": "runBackup",
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
            "name": "updatedAt",
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
    "name": "backupRunMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "backupRunMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "4c9d5130fe90c6451069768ca915bd30",
    "id": null,
    "metadata": {},
    "name": "backupRunMutation",
    "operationKind": "mutation",
    "text": "mutation backupRunMutation {\n  runBackup {\n    settings {\n      updatedAt\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "e7cf022754829ca949eaa1fa70da3ab1";

export default node;
