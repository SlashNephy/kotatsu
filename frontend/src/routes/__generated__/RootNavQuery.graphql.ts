/**
 * @generated SignedSource<<70ccf441d1ce69ee3d9ff7086e6efe53>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RootNavQuery$variables = Record<PropertyKey, never>;
export type RootNavQuery$data = {
  readonly animeRecordSettings: {
    readonly enabled: boolean;
  };
  readonly systemSettings: {
    readonly dramaLinkingEnabled: boolean;
  };
};
export type RootNavQuery = {
  response: RootNavQuery$data;
  variables: RootNavQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "AnimeRecordSettings",
    "kind": "LinkedField",
    "name": "animeRecordSettings",
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
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemSettings",
    "kind": "LinkedField",
    "name": "systemSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "dramaLinkingEnabled",
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
    "name": "RootNavQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "RootNavQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "fe08767b456e6bf929ab075be67b7d7b",
    "id": null,
    "metadata": {},
    "name": "RootNavQuery",
    "operationKind": "query",
    "text": "query RootNavQuery {\n  animeRecordSettings {\n    enabled\n  }\n  systemSettings {\n    dramaLinkingEnabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "50d6238aad4ffe0d915f20852804e0e6";

export default node;
