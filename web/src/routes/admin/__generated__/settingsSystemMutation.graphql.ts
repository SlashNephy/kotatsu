/**
 * @generated SignedSource<<613c9427570e502c5fc9560d38063eb9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type SystemSettingsInput = {
  archiveRetentionDays?: number | null | undefined;
  recordingDefaults?: RecordingSettingsOverridesInput | null | undefined;
};
export type RecordingSettingsOverridesInput = {
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
};
export type settingsSystemMutation$variables = {
  input: SystemSettingsInput;
};
export type settingsSystemMutation$data = {
  readonly updateSystemSettings: {
    readonly updatedAt: string | null | undefined;
  };
};
export type settingsSystemMutation = {
  response: settingsSystemMutation$data;
  variables: settingsSystemMutation$variables;
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
    "concreteType": "SystemSettings",
    "kind": "LinkedField",
    "name": "updateSystemSettings",
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
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "settingsSystemMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "settingsSystemMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "1e067190f01c9b3602d9672ca35ec0d4",
    "id": null,
    "metadata": {},
    "name": "settingsSystemMutation",
    "operationKind": "mutation",
    "text": "mutation settingsSystemMutation(\n  $input: SystemSettingsInput!\n) {\n  updateSystemSettings(input: $input) {\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "ad262ffbbd80e2be77ea45de8874f677";

export default node;
