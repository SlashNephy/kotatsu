/**
 * @generated SignedSource<<77d45cc1e2eb816c5109b612f2cbc158>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type accountBeginMutation$variables = {
  destination: AnimeRecordDestination;
};
export type accountBeginMutation$data = {
  readonly beginAnimeRecordConnection: string;
};
export type accountBeginMutation = {
  response: accountBeginMutation$data;
  variables: accountBeginMutation$variables;
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
    "kind": "ScalarField",
    "name": "beginAnimeRecordConnection",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "accountBeginMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "accountBeginMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "20f391ebfbd5710e049690b3d2dd2e00",
    "id": null,
    "metadata": {},
    "name": "accountBeginMutation",
    "operationKind": "mutation",
    "text": "mutation accountBeginMutation(\n  $destination: AnimeRecordDestination!\n) {\n  beginAnimeRecordConnection(destination: $destination)\n}\n"
  }
};
})();

(node as any).hash = "d91d69232e17f8fdc721e55944f45ad1";

export default node;
