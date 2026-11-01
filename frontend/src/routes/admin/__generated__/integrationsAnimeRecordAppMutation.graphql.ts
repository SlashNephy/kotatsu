/**
 * @generated SignedSource<<88a3b232c1c73494fe3e22ae7b38be87>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeRecordDestination = "ANILIST" | "ANNICT" | "%future added value";
export type AnimeRecordAppInput = {
  clientId: string;
  clientSecret?: string | null | undefined;
};
export type integrationsAnimeRecordAppMutation$variables = {
  destination: AnimeRecordDestination;
  input: AnimeRecordAppInput;
};
export type integrationsAnimeRecordAppMutation$data = {
  readonly updateAnimeRecordApp: {
    readonly clientId: string | null | undefined;
    readonly configured: boolean;
    readonly destination: AnimeRecordDestination;
  };
};
export type integrationsAnimeRecordAppMutation = {
  response: integrationsAnimeRecordAppMutation$data;
  variables: integrationsAnimeRecordAppMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "destination"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
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
      },
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "AnimeRecordApp",
    "kind": "LinkedField",
    "name": "updateAnimeRecordApp",
    "plural": false,
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
        "name": "clientId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "configured",
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
    "name": "integrationsAnimeRecordAppMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsAnimeRecordAppMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f5a82224c869875c47df261ccdeb311c",
    "id": null,
    "metadata": {},
    "name": "integrationsAnimeRecordAppMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsAnimeRecordAppMutation(\n  $destination: AnimeRecordDestination!\n  $input: AnimeRecordAppInput!\n) {\n  updateAnimeRecordApp(destination: $destination, input: $input) {\n    destination\n    clientId\n    configured\n  }\n}\n"
  }
};
})();

(node as any).hash = "a1a8e994464437b59dc048f1ac878c75";

export default node;
