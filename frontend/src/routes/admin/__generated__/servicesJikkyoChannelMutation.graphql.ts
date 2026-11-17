/**
 * @generated SignedSource<<66ce1b6d843b638a853cb47bf5335a96>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CMDetector = "FFMPEG" | "LOGO" | "%future added value";
export type CommentSource = "NICOLOG" | "NX_JIKKYO" | "%future added value";
export type SystemSettingsInput = {
  animeLinkingEnabled?: boolean | null | undefined;
  archiveRetentionDays?: number | null | undefined;
  auditClientInfoRetentionDays?: number | null | undefined;
  auditRetentionDays?: number | null | undefined;
  captionThreads?: number | null | undefined;
  cmDetector?: CMDetector | null | undefined;
  commentSources?: ReadonlyArray<CommentSource> | null | undefined;
  dataBroadcastIpEnabled?: boolean | null | undefined;
  dramaLinkingEnabled?: boolean | null | undefined;
  drcsReplacements?: ReadonlyArray<DrcsReplacementInput> | null | undefined;
  epgstationApiEnabled?: boolean | null | undefined;
  externalPlayerUrlsEnabled?: boolean | null | undefined;
  hibpEnabled?: boolean | null | undefined;
  iptvFeedsEnabled?: boolean | null | undefined;
  jikkyoChannels?: ReadonlyArray<JikkyoChannelOverrideInput> | null | undefined;
  mcpEnabled?: boolean | null | undefined;
  networkActivityEnabled?: boolean | null | undefined;
  networkActivityRetentionDays?: number | null | undefined;
  notificationRetentionDays?: number | null | undefined;
  recordingDefaults?: RecordingSettingsOverridesInput | null | undefined;
  recordingDownloadsEnabled?: boolean | null | undefined;
  ruleDefaults?: RuleDefaultsInput | null | undefined;
  ruleUnmatchedGraceMinutes?: number | null | undefined;
  sleepControlEnabled?: boolean | null | undefined;
  thumbnailPreviewsEnabled?: boolean | null | undefined;
  timeshiftWhileWatching?: boolean | null | undefined;
  timeshiftWhileWatchingRetentionSeconds?: number | null | undefined;
  tvtplayChaptersEnabled?: boolean | null | undefined;
  watchProgressRetentionDays?: number | null | undefined;
  webPushSubject?: string | null | undefined;
};
export type RecordingSettingsOverridesInput = {
  cmDetector?: CMDetector | null | undefined;
  detectCm?: boolean | null | undefined;
  endMarginSeconds?: number | null | undefined;
  fileNameTemplate?: string | null | undefined;
  generateCaptions?: boolean | null | undefined;
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
export type JikkyoChannelOverrideInput = {
  channelId?: string | null | undefined;
  networkId: number;
  serviceId: number;
};
export type servicesJikkyoChannelMutation$variables = {
  input: SystemSettingsInput;
};
export type servicesJikkyoChannelMutation$data = {
  readonly updateSystemSettings: {
    readonly jikkyoChannels: ReadonlyArray<{
      readonly channelId: string;
      readonly networkId: number;
      readonly serviceId: number;
    }>;
  };
};
export type servicesJikkyoChannelMutation = {
  response: servicesJikkyoChannelMutation$data;
  variables: servicesJikkyoChannelMutation$variables;
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
        "concreteType": "JikkyoChannelOverride",
        "kind": "LinkedField",
        "name": "jikkyoChannels",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "networkId",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "serviceId",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "channelId",
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
    "name": "servicesJikkyoChannelMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "servicesJikkyoChannelMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "77361c234e36bd309408bf98beee27ff",
    "id": null,
    "metadata": {},
    "name": "servicesJikkyoChannelMutation",
    "operationKind": "mutation",
    "text": "mutation servicesJikkyoChannelMutation(\n  $input: SystemSettingsInput!\n) {\n  updateSystemSettings(input: $input) {\n    jikkyoChannels {\n      networkId\n      serviceId\n      channelId\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "84c4d828e86765c3f9936c6276087a50";

export default node;
