/**
 * @generated SignedSource<<d65909af03874ef953a236a45d9b0298>>
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
  externalPlayerUrlsEnabled?: boolean | null | undefined;
  iptvFeedsEnabled?: boolean | null | undefined;
  mcpEnabled?: boolean | null | undefined;
  networkActivityEnabled?: boolean | null | undefined;
  networkActivityRetentionDays?: number | null | undefined;
  notificationRetentionDays?: number | null | undefined;
  recordingDefaults?: RecordingSettingsOverridesInput | null | undefined;
  recordingDownloadsEnabled?: boolean | null | undefined;
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
  transcodeFrameRate?: string | null | undefined;
  transcodePreset?: string | null | undefined;
};
export type RuleDefaultsInput = {
  avoidDuplicates?: boolean | null | undefined;
  bundleSameEpisodes?: boolean | null | undefined;
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
