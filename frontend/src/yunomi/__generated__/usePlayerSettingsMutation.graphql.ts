/**
 * @generated SignedSource<<5620af01d9b0417d483ea9f15fcec699>>
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
export type usePlayerSettingsMutation$variables = {
  input: PlayerSettingsInput;
};
export type usePlayerSettingsMutation$data = {
  readonly updatePlayerSettings: {
    readonly captions: {
      readonly backgroundOpacity: number | null | undefined;
      readonly stroke: boolean;
      readonly superimposeLive: boolean;
      readonly superimposeRecorded: boolean;
    };
    readonly captionsEnabled: boolean;
    readonly commentSources: ReadonlyArray<CommentSource>;
    readonly comments: {
      readonly mixOtherBroadcasts: boolean;
      readonly mutedCommands: ReadonlyArray<string>;
      readonly mutedKeywords: ReadonlyArray<{
        readonly match: KeywordMuteMatch;
        readonly text: string;
      }>;
      readonly mutedUsers: ReadonlyArray<string>;
      readonly opacity: number;
      readonly scale: number;
      readonly showAll: boolean;
      readonly speed: number;
    };
    readonly jikkyoEnabled: boolean;
  };
};
export type usePlayerSettingsMutation = {
  response: usePlayerSettingsMutation$data;
  variables: usePlayerSettingsMutation$variables;
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
        "kind": "ScalarField",
        "name": "jikkyoEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "commentSources",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "captionsEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "CommentSettings",
        "kind": "LinkedField",
        "name": "comments",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "opacity",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "speed",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "scale",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "showAll",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "MutedKeyword",
            "kind": "LinkedField",
            "name": "mutedKeywords",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "text",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "match",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "mutedUsers",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "mutedCommands",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "mixOtherBroadcasts",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "CaptionSettings",
        "kind": "LinkedField",
        "name": "captions",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "superimposeLive",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "superimposeRecorded",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "stroke",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "backgroundOpacity",
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
    "name": "usePlayerSettingsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "usePlayerSettingsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "971205b8c4417c5dc9f5e1bd192b9435",
    "id": null,
    "metadata": {},
    "name": "usePlayerSettingsMutation",
    "operationKind": "mutation",
    "text": "mutation usePlayerSettingsMutation(\n  $input: PlayerSettingsInput!\n) {\n  updatePlayerSettings(input: $input) {\n    jikkyoEnabled\n    commentSources\n    captionsEnabled\n    comments {\n      opacity\n      speed\n      scale\n      showAll\n      mutedKeywords {\n        text\n        match\n      }\n      mutedUsers\n      mutedCommands\n      mixOtherBroadcasts\n    }\n    captions {\n      superimposeLive\n      superimposeRecorded\n      stroke\n      backgroundOpacity\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "aef7caa6c219fa60a901b96c29a25e25";

export default node;
