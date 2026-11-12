/**
 * @generated SignedSource<<8405c4a69dbb8d56926f09976cb2b7b7>>
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
export type timeshiftWhileWatchingMutation$variables = {
  input: SystemSettingsInput;
};
export type timeshiftWhileWatchingMutation$data = {
  readonly updateSystemSettings: {
    readonly timeshiftWhileWatching: boolean;
    readonly timeshiftWhileWatchingRetentionSeconds: number;
  };
};
export type timeshiftWhileWatchingMutation = {
  response: timeshiftWhileWatchingMutation$data;
  variables: timeshiftWhileWatchingMutation$variables;
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
        "name": "timeshiftWhileWatching",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "timeshiftWhileWatchingRetentionSeconds",
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
    "name": "timeshiftWhileWatchingMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "timeshiftWhileWatchingMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "db1249fb17076e518fe1401c5596a179",
    "id": null,
    "metadata": {},
    "name": "timeshiftWhileWatchingMutation",
    "operationKind": "mutation",
    "text": "mutation timeshiftWhileWatchingMutation(\n  $input: SystemSettingsInput!\n) {\n  updateSystemSettings(input: $input) {\n    timeshiftWhileWatching\n    timeshiftWhileWatchingRetentionSeconds\n  }\n}\n"
  }
};
})();

(node as any).hash = "0e154fbb32f90989c6493da2d0d01339";

export default node;
