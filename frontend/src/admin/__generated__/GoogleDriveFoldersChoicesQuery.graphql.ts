/**
 * @generated SignedSource<<3a02bc6d1b0ac412520c4d2d8c721b1c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type GoogleDriveFoldersChoicesQuery$variables = {
  parentId?: string | null | undefined;
};
export type GoogleDriveFoldersChoicesQuery$data = {
  readonly googleDriveFolderChoices: ReadonlyArray<{
    readonly id: string;
    readonly name: string;
  }>;
};
export type GoogleDriveFoldersChoicesQuery = {
  response: GoogleDriveFoldersChoicesQuery$data;
  variables: GoogleDriveFoldersChoicesQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "parentId"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "parentId",
        "variableName": "parentId"
      }
    ],
    "concreteType": "GoogleDriveFolderChoice",
    "kind": "LinkedField",
    "name": "googleDriveFolderChoices",
    "plural": true,
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
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "GoogleDriveFoldersChoicesQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "GoogleDriveFoldersChoicesQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "a3292f473553ddff187cdc192fae2a9f",
    "id": null,
    "metadata": {},
    "name": "GoogleDriveFoldersChoicesQuery",
    "operationKind": "query",
    "text": "query GoogleDriveFoldersChoicesQuery(\n  $parentId: String\n) {\n  googleDriveFolderChoices(parentId: $parentId) {\n    id\n    name\n  }\n}\n"
  }
};
})();

(node as any).hash = "91bbbac62e80dfd782945a2ce62ed00d";

export default node;
