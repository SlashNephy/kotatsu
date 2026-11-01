/**
 * @generated SignedSource<<1276fee4993b14e63f256207febdd223>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type SystemSettingsInput = {
  archiveRetentionDays?: number | null | undefined;
  dataBroadcastIpEnabled?: boolean | null | undefined;
  drcsReplacements?: ReadonlyArray<DrcsReplacementInput> | null | undefined;
  mcpEnabled?: boolean | null | undefined;
  notificationRetentionDays?: number | null | undefined;
  recordingDefaults?: RecordingSettingsOverridesInput | null | undefined;
  ruleDefaults?: RuleDefaultsInput | null | undefined;
};
export type RecordingSettingsOverridesInput = {
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
};
export type RuleDefaultsInput = {
  avoidDuplicates?: boolean | null | undefined;
  clearDuplicateWindowDays?: boolean;
  duplicateWindowDays?: number | null | undefined;
  followEventRelay?: boolean | null | undefined;
};
export type DrcsReplacementInput = {
  md5: string;
  text?: string | null | undefined;
};
export type integrationsSystemSettingsMutation$variables = {
  input: SystemSettingsInput;
};
export type integrationsSystemSettingsMutation$data = {
  readonly updateSystemSettings: {
    readonly updatedAt: string | null | undefined;
  };
};
export type integrationsSystemSettingsMutation = {
  response: integrationsSystemSettingsMutation$data;
  variables: integrationsSystemSettingsMutation$variables;
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
    "name": "integrationsSystemSettingsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsSystemSettingsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "a12b25c4ee4abc9a18567796c8c69402",
    "id": null,
    "metadata": {},
    "name": "integrationsSystemSettingsMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsSystemSettingsMutation(\n  $input: SystemSettingsInput!\n) {\n  updateSystemSettings(input: $input) {\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "7072b84396d4d91cdbf88a8328f88ccb";

export default node;
