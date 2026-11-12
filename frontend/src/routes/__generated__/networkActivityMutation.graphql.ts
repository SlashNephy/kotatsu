/**
 * @generated SignedSource<<a5943b89feeb6aadcca2a97c632b2a18>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CommentSource = "NICOLOG" | "NX_JIKKYO" | "%future added value";
export type SystemSettingsInput = {
  animeLinkingEnabled?: boolean | null | undefined;
  archiveRetentionDays?: number | null | undefined;
  commentSources?: ReadonlyArray<CommentSource> | null | undefined;
  dataBroadcastIpEnabled?: boolean | null | undefined;
  drcsReplacements?: ReadonlyArray<DrcsReplacementInput> | null | undefined;
  mcpEnabled?: boolean | null | undefined;
  networkActivityEnabled?: boolean | null | undefined;
  networkActivityRetentionDays?: number | null | undefined;
  notificationRetentionDays?: number | null | undefined;
  recordingDefaults?: RecordingSettingsOverridesInput | null | undefined;
  ruleDefaults?: RuleDefaultsInput | null | undefined;
  ruleUnmatchedGraceMinutes?: number | null | undefined;
  timeshiftWhileWatching?: boolean | null | undefined;
  timeshiftWhileWatchingRetentionSeconds?: number | null | undefined;
  watchProgressRetentionDays?: number | null | undefined;
};
export type RecordingSettingsOverridesInput = {
  detectCm?: boolean | null | undefined;
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  joinSegments?: boolean | null | undefined;
  poolId?: string | null | undefined;
  priority?: number | null | undefined;
  startMarginSeconds?: number | null | undefined;
  transcodePreset?: string | null | undefined;
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
export type networkActivityMutation$variables = {
  input: SystemSettingsInput;
};
export type networkActivityMutation$data = {
  readonly updateSystemSettings: {
    readonly networkActivityEnabled: boolean;
  };
};
export type networkActivityMutation = {
  response: networkActivityMutation$data;
  variables: networkActivityMutation$variables;
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
        "name": "networkActivityEnabled",
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
    "name": "networkActivityMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "networkActivityMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "5f01006afe4f98934237ac1069ebf84b",
    "id": null,
    "metadata": {},
    "name": "networkActivityMutation",
    "operationKind": "mutation",
    "text": "mutation networkActivityMutation(\n  $input: SystemSettingsInput!\n) {\n  updateSystemSettings(input: $input) {\n    networkActivityEnabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "145c49e684afa32a4bc53f89a883efa1";

export default node;
