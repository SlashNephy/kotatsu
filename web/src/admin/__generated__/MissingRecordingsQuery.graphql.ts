/**
 * @generated SignedSource<<8e5e0eebd501bd178a6e94cbfeffe3ba>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type MissingRecordingsQuery$variables = Record<PropertyKey, never>;
export type MissingRecordingsQuery$data = {
  readonly missingRecordings: ReadonlyArray<{
    readonly missingSegments: number;
    readonly recording: {
      readonly id: string;
      readonly name: string;
      readonly protected: boolean;
      readonly recordedFrom: string;
    };
    readonly segments: number;
  }>;
};
export type MissingRecordingsQuery = {
  response: MissingRecordingsQuery$data;
  variables: MissingRecordingsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "MissingRecording",
    "kind": "LinkedField",
    "name": "missingRecordings",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "Recording",
        "kind": "LinkedField",
        "name": "recording",
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
            "name": "name",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "recordedFrom",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "protected",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "segments",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "missingSegments",
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
    "name": "MissingRecordingsQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "MissingRecordingsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "2e40165c3a04e57637ff0fac1eb2113c",
    "id": null,
    "metadata": {},
    "name": "MissingRecordingsQuery",
    "operationKind": "query",
    "text": "query MissingRecordingsQuery {\n  missingRecordings {\n    recording {\n      id\n      name\n      recordedFrom\n      protected\n    }\n    segments\n    missingSegments\n  }\n}\n"
  }
};
})();

(node as any).hash = "0742e8840eb8043ff3519a357d3c79ab";

export default node;
