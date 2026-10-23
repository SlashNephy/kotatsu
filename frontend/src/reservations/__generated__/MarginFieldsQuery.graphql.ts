/**
 * @generated SignedSource<<3d940682023bd67c9f7317d80e54f92d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type MarginFieldsQuery$variables = Record<PropertyKey, never>;
export type MarginFieldsQuery$data = {
  readonly systemSettings: {
    readonly recordingDefaults: {
      readonly endMarginSeconds: number;
      readonly startMarginSeconds: number;
    };
  };
};
export type MarginFieldsQuery = {
  response: MarginFieldsQuery$data;
  variables: MarginFieldsQuery$variables;
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
        "concreteType": "RecordingSettings",
        "kind": "LinkedField",
        "name": "recordingDefaults",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "startMarginSeconds",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "endMarginSeconds",
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
    "name": "MarginFieldsQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "MarginFieldsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "ea84bede96115b8961dbb1e2acf116fe",
    "id": null,
    "metadata": {},
    "name": "MarginFieldsQuery",
    "operationKind": "query",
    "text": "query MarginFieldsQuery {\n  systemSettings {\n    recordingDefaults {\n      startMarginSeconds\n      endMarginSeconds\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "0b9391a128818feff6d9ee09b5f0c5e7";

export default node;
