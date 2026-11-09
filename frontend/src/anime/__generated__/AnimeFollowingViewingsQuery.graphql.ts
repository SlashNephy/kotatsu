/**
 * @generated SignedSource<<17248cbd4f8962fa00b78ab0c6ba28bb>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeFollowingStatus = "ON_HOLD" | "REPEATING" | "STOP_WATCHING" | "WANNA_WATCH" | "WATCHED" | "WATCHING" | "%future added value";
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type AnimeFollowingViewingsQuery$variables = {
  tid: number;
};
export type AnimeFollowingViewingsQuery$data = {
  readonly animeFollowingViewings: {
    readonly enabled: boolean;
    readonly failed: boolean;
    readonly needsRefresh: boolean;
    readonly viewings: ReadonlyArray<{
      readonly avatarUrl: string | null | undefined;
      readonly destination: AnimeRecordDestination;
      readonly episodesCount: number | null | undefined;
      readonly name: string;
      readonly notes: string | null | undefined;
      readonly profileUrl: string;
      readonly progress: number | null | undefined;
      readonly score: number | null | undefined;
      readonly status: AnimeFollowingStatus;
      readonly username: string;
    }>;
  };
};
export type AnimeFollowingViewingsQuery = {
  response: AnimeFollowingViewingsQuery$data;
  variables: AnimeFollowingViewingsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "tid"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "tid",
        "variableName": "tid"
      }
    ],
    "concreteType": "AnimeFollowingViewings",
    "kind": "LinkedField",
    "name": "animeFollowingViewings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "enabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "needsRefresh",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "failed",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "AnimeFollowingViewing",
        "kind": "LinkedField",
        "name": "viewings",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "destination",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "username",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "name",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "avatarUrl",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "profileUrl",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "status",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "progress",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "episodesCount",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "score",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "notes",
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
    "name": "AnimeFollowingViewingsQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "AnimeFollowingViewingsQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "01dec58c46aa559ec76e5b4c7c4c9471",
    "id": null,
    "metadata": {},
    "name": "AnimeFollowingViewingsQuery",
    "operationKind": "query",
    "text": "query AnimeFollowingViewingsQuery(\n  $tid: Int!\n) {\n  animeFollowingViewings(tid: $tid) {\n    enabled\n    needsRefresh\n    failed\n    viewings {\n      destination\n      username\n      name\n      avatarUrl\n      profileUrl\n      status\n      progress\n      episodesCount\n      score\n      notes\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "70bf5dff8e414b8f6d93f346ef8fe266";

export default node;
