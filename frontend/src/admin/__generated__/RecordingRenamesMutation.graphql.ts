/**
 * @generated SignedSource<<f94bb8aa69e1005e880fe257304917fc>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingRenamesMutation$variables = {
  ids: ReadonlyArray<string>;
};
export type RecordingRenamesMutation$data = {
  readonly renameRecordingFiles: {
    readonly failed: number;
    readonly renamed: number;
    readonly skipped: number;
  };
};
export type RecordingRenamesMutation = {
  response: RecordingRenamesMutation$data;
  variables: RecordingRenamesMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "ids"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "ids",
        "variableName": "ids"
      }
    ],
    "concreteType": "RenameRecordingFilesPayload",
    "kind": "LinkedField",
    "name": "renameRecordingFiles",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "renamed",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "skipped",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "failed",
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
    "name": "RecordingRenamesMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingRenamesMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "e6ddc65790361532a5c8e1a80222d610",
    "id": null,
    "metadata": {},
    "name": "RecordingRenamesMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingRenamesMutation(\n  $ids: [ID!]!\n) {\n  renameRecordingFiles(ids: $ids) {\n    renamed\n    skipped\n    failed\n  }\n}\n"
  }
};
})();

(node as any).hash = "53819915f46951cc6c8f8c9041dfb0db";

export default node;
