/**
 * @generated SignedSource<<7d656a51353c5b5679afaa0e1697fdee>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type BackupSettingsInput = {
  generations?: number | null | undefined;
  intervalHours?: number | null | undefined;
  recipients?: ReadonlyArray<string> | null | undefined;
};
export type backupMutation$variables = {
  input: BackupSettingsInput;
};
export type backupMutation$data = {
  readonly updateBackupSettings: {
    readonly settings: {
      readonly updatedAt: string | null | undefined;
    };
  };
};
export type backupMutation = {
  response: backupMutation$data;
  variables: backupMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "BackupStatus",
    "kind": "LinkedField",
    "name": "updateBackupSettings",
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
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "backupMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "backupMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "fe605369e505b8fc4d15abf7ebc31473",
    "id": null,
    "metadata": {},
    "name": "backupMutation",
    "operationKind": "mutation",
    "text": "mutation backupMutation(\n  $input: BackupSettingsInput!\n) {\n  updateBackupSettings(input: $input) {\n    settings {\n      updatedAt\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "a30d00a679bc353fa682b749d5077a61";

export default node;
