/**
 * @generated SignedSource<<7b2a2958f097853720a8229241d99550>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type KeywordMuteMatch = "EXACT" | "PARTIAL" | "PREFIX" | "REGEX" | "SUFFIX" | "%future added value";
export type usePlayerSettingsQuery$variables = Record<PropertyKey, never>;
export type usePlayerSettingsQuery$data = {
  readonly playerSettings: {
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
export type usePlayerSettingsQuery = {
  response: usePlayerSettingsQuery$data;
  variables: usePlayerSettingsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "PlayerSettings",
    "kind": "LinkedField",
    "name": "playerSettings",
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
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "usePlayerSettingsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "8e6d239d666fde78e83e593356eecf63",
    "id": null,
    "metadata": {},
    "name": "usePlayerSettingsQuery",
    "operationKind": "query",
    "text": "query usePlayerSettingsQuery {\n  playerSettings {\n    jikkyoEnabled\n    captionsEnabled\n    comments {\n      opacity\n      speed\n      scale\n      showAll\n      mutedKeywords {\n        text\n        match\n      }\n      mutedUsers\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "3df20b488020a487f2a1eed6c4d63db9";

export default node;
