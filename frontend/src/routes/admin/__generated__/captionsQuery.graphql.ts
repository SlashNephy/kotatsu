/**
 * @generated SignedSource<<d21e59dafb6b57fccc1ee975af46c624>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type captionsQuery$variables = Record<PropertyKey, never>;
export type captionsQuery$data = {
  readonly systemSettings: {
    readonly drcsReplacements: ReadonlyArray<{
      readonly md5: string;
      readonly text: string;
    }>;
    readonly updatedAt: string | null | undefined;
  };
};
export type captionsQuery = {
  response: captionsQuery$data;
  variables: captionsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
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
        "concreteType": "DrcsReplacement",
        "kind": "LinkedField",
        "name": "drcsReplacements",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "md5",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "text",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "updatedAt",
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
    "name": "captionsQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "captionsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "1bfb7e568798cdc5eb2e6fc5407825a6",
    "id": null,
    "metadata": {},
    "name": "captionsQuery",
    "operationKind": "query",
    "text": "query captionsQuery {\n  systemSettings {\n    drcsReplacements {\n      md5\n      text\n    }\n    updatedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "ab4e5d4a90fd1703f32e7d6fb8c2ec85";

export default node;
