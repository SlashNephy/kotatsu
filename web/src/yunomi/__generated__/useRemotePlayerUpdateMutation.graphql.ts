/**
 * @generated SignedSource<<9a96c65ccfe08ca79d38f6db128fd908>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type PlayerKind = "LIVE" | "RECORDING" | "%future added value";
export type PlayerStateInput = {
  audio?: string | null | undefined;
  audios: ReadonlyArray<PlayerAudioInput>;
  dataBroadcast: boolean;
  duration?: number | null | undefined;
  keyGroups: ReadonlyArray<string>;
  kind: PlayerKind;
  muted: boolean;
  paused: boolean;
  position?: number | null | undefined;
  target: string;
  title?: string | null | undefined;
  volume: number;
};
export type PlayerAudioInput = {
  id: string;
  label: string;
};
export type useRemotePlayerUpdateMutation$variables = {
  announce: boolean;
  id: string;
  state: PlayerStateInput;
};
export type useRemotePlayerUpdateMutation$data = {
  readonly updatePlayer: {
    readonly id: string;
  };
};
export type useRemotePlayerUpdateMutation = {
  response: useRemotePlayerUpdateMutation$data;
  variables: useRemotePlayerUpdateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "announce"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "id"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "state"
},
v3 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "announce",
        "variableName": "announce"
      },
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      },
      {
        "kind": "Variable",
        "name": "state",
        "variableName": "state"
      }
    ],
    "concreteType": "Player",
    "kind": "LinkedField",
    "name": "updatePlayer",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "useRemotePlayerUpdateMutation",
    "selections": (v3/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "useRemotePlayerUpdateMutation",
    "selections": (v3/*:: as any*/)
  },
  "params": {
    "cacheID": "8c743b11e6004567647ac1e0ed20baf6",
    "id": null,
    "metadata": {},
    "name": "useRemotePlayerUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation useRemotePlayerUpdateMutation(\n  $id: ID!\n  $state: PlayerStateInput!\n  $announce: Boolean!\n) {\n  updatePlayer(id: $id, state: $state, announce: $announce) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "62e69a44eedecb8410057b5cd7ca2194";

export default node;
