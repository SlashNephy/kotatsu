/**
 * @generated SignedSource<<d46a96046b8e1ef76f1134d74dc38a7f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CommentSource = "NICOLOG" | "NX_JIKKYO" | "%future added value";
export type KeywordMuteMatch = "EXACT" | "PARTIAL" | "PREFIX" | "REGEX" | "SUFFIX" | "%future added value";
export type usePlayerSettingsQuery$variables = Record<PropertyKey, never>;
export type usePlayerSettingsQuery$data = {
  readonly playerSettings: {
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
  readonly systemSettings: {
    readonly commentSources: ReadonlyArray<CommentSource>;
    readonly jikkyoEnabled: boolean;
  };
};
export type usePlayerSettingsQuery = {
  response: usePlayerSettingsQuery$data;
  variables: usePlayerSettingsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "jikkyoEnabled",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "commentSources",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemSettings",
    "kind": "LinkedField",
    "name": "systemSettings",
    "plural": false,
    "selections": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "PlayerSettings",
    "kind": "LinkedField",
    "name": "playerSettings",
    "plural": false,
    "selections": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
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
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "usePlayerSettingsQuery",
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "usePlayerSettingsQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "c31f1a1d71033e1fa7ec87eaba9d7d4f",
    "id": null,
    "metadata": {},
    "name": "usePlayerSettingsQuery",
    "operationKind": "query",
    "text": "query usePlayerSettingsQuery {\n  systemSettings {\n    jikkyoEnabled\n    commentSources\n  }\n  playerSettings {\n    jikkyoEnabled\n    commentSources\n    captionsEnabled\n    comments {\n      opacity\n      speed\n      scale\n      showAll\n      mutedKeywords {\n        text\n        match\n      }\n      mutedUsers\n      mutedCommands\n      mixOtherBroadcasts\n    }\n    captions {\n      superimposeLive\n      superimposeRecorded\n      stroke\n      backgroundOpacity\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "f18c924ab7a89f695bbdc3fbf561622c";

export default node;
