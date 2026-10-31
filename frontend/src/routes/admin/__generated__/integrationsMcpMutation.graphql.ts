/**
 * @generated SignedSource<<b465d195f96f478c3fc13dd0689ecc0f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type SystemSettingsInput = {
  archiveRetentionDays?: number | null | undefined;
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
  text: string;
};
export type integrationsMcpMutation$variables = {
  input: SystemSettingsInput;
};
export type integrationsMcpMutation$data = {
  readonly updateSystemSettings: {
    readonly updatedAt: string | null | undefined;
  };
};
export type integrationsMcpMutation = {
  response: integrationsMcpMutation$data;
  variables: integrationsMcpMutation$variables;
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
    "name": "integrationsMcpMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsMcpMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "2296fead3f40bad62badcf75a371b001",
    "id": null,
    "metadata": {},
    "name": "integrationsMcpMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsMcpMutation(\n  $input: SystemSettingsInput!\n) {\n  updateSystemSettings(input: $input) {\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "90533a7b67f482a04152ac5e5ec5a5b1";

export default node;
