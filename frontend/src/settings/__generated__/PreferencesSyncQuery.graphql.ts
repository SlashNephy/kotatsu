/**
 * @generated SignedSource<<aee22c89ba20c7b73de5f747d90ab5ad>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type PreferencesSyncQuery$variables = Record<PropertyKey, never>;
export type PreferencesSyncQuery$data = {
  readonly playerSettings: {
    readonly preferences: ReadonlyArray<{
      readonly key: string;
      readonly value: string;
    }>;
  };
};
export type PreferencesSyncQuery = {
  response: PreferencesSyncQuery$data;
  variables: PreferencesSyncQuery$variables;
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
        "concreteType": "Preference",
        "kind": "LinkedField",
        "name": "preferences",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "key",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "value",
            "storageKey": null
          }
        ],
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
    "name": "PreferencesSyncQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "PreferencesSyncQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "7f3eeb1d870edc69ff0606569f6637f2",
    "id": null,
    "metadata": {},
    "name": "PreferencesSyncQuery",
    "operationKind": "query",
    "text": "query PreferencesSyncQuery {\n  playerSettings {\n    preferences {\n      key\n      value\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "249bd5e8bf591fbee43badc83ed91177";

export default node;
