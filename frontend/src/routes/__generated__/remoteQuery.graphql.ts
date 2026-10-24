/**
 * @generated SignedSource<<a6324c0bfad5c69d77151ab124f19644>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type PlayerKind = "LIVE" | "RECORDING" | "%future added value";
export type remoteQuery$variables = Record<PropertyKey, never>;
export type remoteQuery$data = {
  readonly players: ReadonlyArray<{
    readonly current: boolean;
    readonly id: string;
    readonly lastSeenAt: string;
    readonly state: {
      readonly audio: string | null | undefined;
      readonly audios: ReadonlyArray<{
        readonly id: string;
        readonly label: string;
      }>;
      readonly dataBroadcast: boolean;
      readonly duration: number | null | undefined;
      readonly keyGroups: ReadonlyArray<string>;
      readonly kind: PlayerKind;
      readonly muted: boolean;
      readonly paused: boolean;
      readonly position: number | null | undefined;
      readonly title: string | null | undefined;
      readonly volume: number;
    };
    readonly userAgent: string;
  }>;
};
export type remoteQuery = {
  response: remoteQuery$data;
  variables: remoteQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v1 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "Player",
    "kind": "LinkedField",
    "name": "players",
    "plural": true,
    "selections": [
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "current",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "userAgent",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "lastSeenAt",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "PlayerState",
        "kind": "LinkedField",
        "name": "state",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "kind",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "title",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "paused",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "position",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "duration",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "volume",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "muted",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "PlayerAudio",
            "kind": "LinkedField",
            "name": "audios",
            "plural": true,
            "selections": [
              (v0/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "label",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "audio",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "dataBroadcast",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "keyGroups",
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
    "name": "remoteQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "remoteQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "25d81db5c828f2a9d671bae5a0c70cbf",
    "id": null,
    "metadata": {},
    "name": "remoteQuery",
    "operationKind": "query",
    "text": "query remoteQuery {\n  players {\n    id\n    current\n    userAgent\n    lastSeenAt\n    state {\n      kind\n      title\n      paused\n      position\n      duration\n      volume\n      muted\n      audios {\n        id\n        label\n      }\n      audio\n      dataBroadcast\n      keyGroups\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "929f762897bf75c4982bfa2a7ea442fc";

export default node;
