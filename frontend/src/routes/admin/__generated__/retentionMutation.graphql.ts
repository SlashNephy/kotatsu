/**
 * @generated SignedSource<<31ed4c22af52b43727395fca58ca432e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type SystemSettingsInput = {
  archiveRetentionDays?: number | null | undefined;
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
export type retentionMutation$variables = {
  input: SystemSettingsInput;
};
export type retentionMutation$data = {
  readonly updateSystemSettings: {
    readonly updatedAt: string | null | undefined;
  };
};
export type retentionMutation = {
  response: retentionMutation$data;
  variables: retentionMutation$variables;
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
    "name": "retentionMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "retentionMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "4dafd3103ed719966c6e3ee9d6bfe497",
    "id": null,
    "metadata": {},
    "name": "retentionMutation",
    "operationKind": "mutation",
    "text": "mutation retentionMutation(\n  $input: SystemSettingsInput!\n) {\n  updateSystemSettings(input: $input) {\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "d3d9711dca14072c4ae0f81936fd2d1f";

export default node;
