/**
 * @generated SignedSource<<edc76b3a49538ff526a4b0ee82741c90>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type MissingRecordingsCleanUpMutation$variables = {
  ids: ReadonlyArray<string>;
};
export type MissingRecordingsCleanUpMutation$data = {
  readonly cleanUpMissingRecordings: {
    readonly deletedRecordings: number;
    readonly prunedSegments: number;
    readonly skipped: number;
  };
};
export type MissingRecordingsCleanUpMutation = {
  response: MissingRecordingsCleanUpMutation$data;
  variables: MissingRecordingsCleanUpMutation$variables;
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
    "concreteType": "CleanUpMissingRecordingsPayload",
    "kind": "LinkedField",
    "name": "cleanUpMissingRecordings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "deletedRecordings",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "prunedSegments",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "skipped",
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
    "name": "MissingRecordingsCleanUpMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "MissingRecordingsCleanUpMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "5fc53e579e000bc6305d34c76d34e242",
    "id": null,
    "metadata": {},
    "name": "MissingRecordingsCleanUpMutation",
    "operationKind": "mutation",
    "text": "mutation MissingRecordingsCleanUpMutation(\n  $ids: [ID!]!\n) {\n  cleanUpMissingRecordings(ids: $ids) {\n    deletedRecordings\n    prunedSegments\n    skipped\n  }\n}\n"
  }
};
})();

(node as any).hash = "6d93a232c59cb933d98a275b8f7bff9a";

export default node;
