/**
 * @generated SignedSource<<e1f2bcdeec456f36a58586eb4118b752>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type accountDisconnectMutation$variables = {
  destination: AnimeRecordDestination;
};
export type accountDisconnectMutation$data = {
  readonly disconnectAnimeRecord: {
    readonly enabled: boolean;
  };
};
export type accountDisconnectMutation = {
  response: accountDisconnectMutation$data;
  variables: accountDisconnectMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "destination"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "destination",
        "variableName": "destination"
      }
    ],
    "concreteType": "AnimeRecordSettings",
    "kind": "LinkedField",
    "name": "disconnectAnimeRecord",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "enabled",
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
    "name": "accountDisconnectMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "accountDisconnectMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ca680432398abc401b411ccacd4bf778",
    "id": null,
    "metadata": {},
    "name": "accountDisconnectMutation",
    "operationKind": "mutation",
    "text": "mutation accountDisconnectMutation(\n  $destination: AnimeRecordDestination!\n) {\n  disconnectAnimeRecord(destination: $destination) {\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "2dea000dbe489b0860a4c4c6c0c46319";

export default node;
