/**
 * @generated SignedSource<<e3785d977d53fa9024a8c7e5a0f50152>>
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
    "cacheID": "3da6b204ccce91c391fc88aaf2dda886",
    "id": null,
    "metadata": {},
    "name": "RootNavQuery",
    "operationKind": "query",
    "text": "query RootNavQuery {\n  animeRecordSettings {\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "83139a113b4c003cf5ef0114fd116977";

export default node;
