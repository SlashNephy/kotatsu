/**
 * @generated SignedSource<<9b826ea44bf69facafc764fda9f5ce20>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type usePlayerSettingsQuery$variables = Record<PropertyKey, never>;
export type usePlayerSettingsQuery$data = {
  readonly playerSettings: {
    readonly captionsEnabled: boolean;
    readonly jikkyoEnabled: boolean;
  };
};
export type usePlayerSettingsQuery = {
  response: usePlayerSettingsQuery$data;
  variables: usePlayerSettingsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "PlayerSettings",
    "kind": "LinkedField",
    "name": "playerSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "jikkyoEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "captionsEnabled",
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
    "name": "usePlayerSettingsQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "usePlayerSettingsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "9c17aa42acb5b99ef2703a12b4c4e8d4",
    "id": null,
    "metadata": {},
    "name": "usePlayerSettingsQuery",
    "operationKind": "query",
    "text": "query usePlayerSettingsQuery {\n  playerSettings {\n    jikkyoEnabled\n    captionsEnabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "e2e9c19298992bb92853e1a4d7b1a8ed";

export default node;
