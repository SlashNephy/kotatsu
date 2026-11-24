/**
 * @generated SignedSource<<03da0ec8d03d316d98ae304ea70250f2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CommentSource = "NICOLOG" | "NX_JIKKYO" | "%future added value";
export type KeywordMuteMatch = "EXACT" | "PARTIAL" | "PREFIX" | "REGEX" | "SUFFIX" | "%future added value";
export type PlayerSettingsInput = {
  captions?: CaptionSettingsInput | null | undefined;
  captionsEnabled?: boolean | null | undefined;
  commentSources?: ReadonlyArray<CommentSource> | null | undefined;
  comments?: CommentSettingsInput | null | undefined;
  jikkyoEnabled?: boolean | null | undefined;
  preferences?: ReadonlyArray<PreferenceInput> | null | undefined;
};
export type CommentSettingsInput = {
  mixOtherBroadcasts: boolean;
  mutedCommands: ReadonlyArray<string>;
  mutedKeywords: ReadonlyArray<MutedKeywordInput>;
  mutedUsers: ReadonlyArray<string>;
  opacity: number;
  scale: number;
  showAll: boolean;
  speed: number;
};
export type MutedKeywordInput = {
  match: KeywordMuteMatch;
  text: string;
};
export type CaptionSettingsInput = {
  backgroundOpacity?: number | null | undefined;
  stroke: boolean;
  superimposeLive: boolean;
  superimposeRecorded: boolean;
};
export type PreferenceInput = {
  key: string;
  value?: string | null | undefined;
};
export type PreferencesSyncMutation$variables = {
  input: PlayerSettingsInput;
};
export type PreferencesSyncMutation$data = {
  readonly updatePlayerSettings: {
    readonly preferences: ReadonlyArray<{
      readonly key: string;
      readonly value: string;
    }>;
  };
};
export type PreferencesSyncMutation = {
  response: PreferencesSyncMutation$data;
  variables: PreferencesSyncMutation$variables;
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
    "concreteType": "PlayerSettings",
    "kind": "LinkedField",
    "name": "updatePlayerSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "Preference",
        "kind": "LinkedField",
        "name": "preferences",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "key",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "value",
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
    "name": "PreferencesSyncMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "PreferencesSyncMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ae7343ff3d58c99745e16df825ce7784",
    "id": null,
    "metadata": {},
    "name": "PreferencesSyncMutation",
    "operationKind": "mutation",
    "text": "mutation PreferencesSyncMutation(\n  $input: PlayerSettingsInput!\n) {\n  updatePlayerSettings(input: $input) {\n    preferences {\n      key\n      value\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "a4a19a71b390368b2d6c7ad392517469";

export default node;
