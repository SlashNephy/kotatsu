/**
 * @generated SignedSource<<5a6394dd8377f29de10066034a848611>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type KeywordMuteMatch = "EXACT" | "PARTIAL" | "PREFIX" | "REGEX" | "SUFFIX" | "%future added value";
export type PlayerSettingsInput = {
  captions?: CaptionSettingsInput | null | undefined;
  captionsEnabled?: boolean | null | undefined;
  comments?: CommentSettingsInput | null | undefined;
  jikkyoEnabled?: boolean | null | undefined;
};
export type CommentSettingsInput = {
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
    readonly comments: {
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
    "cacheID": "efdf1cedafd9046ff7e120cf5a9d1c44",
    "id": null,
    "metadata": {},
    "name": "usePlayerSettingsMutation",
    "operationKind": "mutation",
    "text": "mutation usePlayerSettingsMutation(\n  $input: PlayerSettingsInput!\n) {\n  updatePlayerSettings(input: $input) {\n    jikkyoEnabled\n    captionsEnabled\n    comments {\n      opacity\n      speed\n      scale\n      showAll\n      mutedKeywords {\n        text\n        match\n      }\n      mutedUsers\n    }\n    captions {\n      superimposeLive\n      superimposeRecorded\n      stroke\n      backgroundOpacity\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "e05a3b1da7b4d580657e505f53c86383";

export default node;
