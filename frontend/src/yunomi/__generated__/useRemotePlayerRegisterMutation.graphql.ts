/**
 * @generated SignedSource<<6b5bd617c17c7fe407d0e5742d502f78>>
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
export type useRemotePlayerRegisterMutation$variables = {
  state: PlayerStateInput;
};
export type useRemotePlayerRegisterMutation$data = {
  readonly registerPlayer: {
    readonly id: string;
  };
};
export type useRemotePlayerRegisterMutation = {
  response: useRemotePlayerRegisterMutation$data;
  variables: useRemotePlayerRegisterMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "state"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "state",
        "variableName": "state"
      }
    ],
    "concreteType": "Player",
    "kind": "LinkedField",
    "name": "registerPlayer",
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
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "useRemotePlayerRegisterMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "useRemotePlayerRegisterMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "39b081ee9eb0c830ac20195770308705",
    "id": null,
    "metadata": {},
    "name": "useRemotePlayerRegisterMutation",
    "operationKind": "mutation",
    "text": "mutation useRemotePlayerRegisterMutation(\n  $state: PlayerStateInput!\n) {\n  registerPlayer(state: $state) {\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "b49c96eada0f4fa831a874dc05acfa0d";

export default node;
