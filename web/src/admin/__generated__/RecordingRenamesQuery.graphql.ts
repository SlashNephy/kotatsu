/**
 * @generated SignedSource<<a47e8609fee8e87373c24f4ef6fdb46d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingRenamesQuery$variables = Record<PropertyKey, never>;
export type RecordingRenamesQuery$data = {
  readonly recordingRenames: ReadonlyArray<{
    readonly files: ReadonlyArray<{
      readonly from: string;
      readonly to: string;
    }>;
    readonly recording: {
      readonly id: string;
      readonly name: string;
    };
  }>;
};
export type RecordingRenamesQuery = {
  response: RecordingRenamesQuery$data;
  variables: RecordingRenamesQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "RecordingRename",
    "kind": "LinkedField",
    "name": "recordingRenames",
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
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "RecordingFileRename",
        "kind": "LinkedField",
        "name": "files",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "from",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "to",
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
    "name": "RecordingRenamesQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "RecordingRenamesQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "78e67d922c8fabf29234711e76816b3f",
    "id": null,
    "metadata": {},
    "name": "RecordingRenamesQuery",
    "operationKind": "query",
    "text": "query RecordingRenamesQuery {\n  recordingRenames {\n    recording {\n      id\n      name\n    }\n    files {\n      from\n      to\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "544bfb28ac3739e199b036a6225a4db6";

export default node;
