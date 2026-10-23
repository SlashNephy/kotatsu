/**
 * @generated SignedSource<<e7bdeb71f4b76be64d837be179c846a6>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type PlayerCommandType = "PAUSE" | "PLAY" | "PRESS_KEY" | "SEEK" | "SELECT_AUDIO" | "SET_MUTED" | "SET_VOLUME" | "STOP" | "%future added value";
export type useRemotePlayerCommandsSubscription$variables = {
  after?: string | null | undefined;
  id: string;
};
export type useRemotePlayerCommandsSubscription$data = {
  readonly playerCommands: {
    readonly audio: string | null | undefined;
    readonly id: string;
    readonly key: string | null | undefined;
    readonly muted: boolean | null | undefined;
    readonly position: number | null | undefined;
    readonly type: PlayerCommandType;
    readonly volume: number | null | undefined;
  };
};
export type useRemotePlayerCommandsSubscription = {
  response: useRemotePlayerCommandsSubscription$data;
  variables: useRemotePlayerCommandsSubscription$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "after"
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
        "name": "after",
        "variableName": "after"
      },
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      }
    ],
    "concreteType": "PlayerCommand",
    "kind": "LinkedField",
    "name": "playerCommands",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "type",
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
        "kind": "ScalarField",
        "name": "audio",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "key",
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
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "useRemotePlayerCommandsSubscription",
    "selections": (v2/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "useRemotePlayerCommandsSubscription",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "20972c9bd7098061818d3cc632f06dec",
    "id": null,
    "metadata": {},
    "name": "useRemotePlayerCommandsSubscription",
    "operationKind": "subscription",
    "text": "subscription useRemotePlayerCommandsSubscription(\n  $id: ID!\n  $after: ID\n) {\n  playerCommands(id: $id, after: $after) {\n    id\n    type\n    position\n    volume\n    muted\n    audio\n    key\n  }\n}\n"
  }
};
})();

(node as any).hash = "dc2738ff111b424a3bbc06954eb5dfc9";

export default node;
