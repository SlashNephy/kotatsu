/**
 * @generated SignedSource<<24d95e95052080c92197d2809f8ea832>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type PlayerCommandType = "PAUSE" | "PLAY" | "PRESS_KEY" | "SEEK" | "SELECT_AUDIO" | "SET_MUTED" | "SET_VOLUME" | "STOP" | "%future added value";
export type PlayerCommandInput = {
  audio?: string | null | undefined;
  key?: string | null | undefined;
  muted?: boolean | null | undefined;
  position?: number | null | undefined;
  type: PlayerCommandType;
  volume?: number | null | undefined;
};
export type remoteCommandMutation$variables = {
  command: PlayerCommandInput;
  id: string;
};
export type remoteCommandMutation$data = {
  readonly sendPlayerCommand: boolean;
};
export type remoteCommandMutation = {
  response: remoteCommandMutation$data;
  variables: remoteCommandMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "command"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "id"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "command",
        "variableName": "command"
      },
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      }
    ],
    "kind": "ScalarField",
    "name": "sendPlayerCommand",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "remoteCommandMutation",
    "selections": (v2/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "remoteCommandMutation",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "18734c17e140da806059ec924210750c",
    "id": null,
    "metadata": {},
    "name": "remoteCommandMutation",
    "operationKind": "mutation",
    "text": "mutation remoteCommandMutation(\n  $id: ID!\n  $command: PlayerCommandInput!\n) {\n  sendPlayerCommand(id: $id, command: $command)\n}\n"
  }
};
})();

(node as any).hash = "a973b9e32e4f10bf12eb6bb847a19399";

export default node;
