/**
 * @generated SignedSource<<8cf70e1e4dd835d74c8360171f194a65>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type libraryDirectoryQuery$variables = {
  library: string;
  path: string;
};
export type libraryDirectoryQuery$data = {
  readonly libraryDirectory: {
    readonly entries: ReadonlyArray<{
      readonly directory: boolean;
      readonly modifiedAt: string;
      readonly name: string;
      readonly path: string;
      readonly size: number;
    }>;
    readonly path: string;
  };
};
export type libraryDirectoryQuery = {
  response: libraryDirectoryQuery$data;
  variables: libraryDirectoryQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "library"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "path"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "path",
  "storageKey": null
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "library",
        "variableName": "library"
      },
      {
        "kind": "Variable",
        "name": "path",
        "variableName": "path"
      }
    ],
    "concreteType": "LibraryDirectory",
    "kind": "LinkedField",
    "name": "libraryDirectory",
    "plural": false,
    "selections": [
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "LibraryEntry",
        "kind": "LinkedField",
        "name": "entries",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "name",
            "storageKey": null
          },
          (v1/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "directory",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "size",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "modifiedAt",
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
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "libraryDirectoryQuery",
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "libraryDirectoryQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "7e723948bd06112bfaa1331a393451c8",
    "id": null,
    "metadata": {},
    "name": "libraryDirectoryQuery",
    "operationKind": "query",
    "text": "query libraryDirectoryQuery(\n  $library: String!\n  $path: String!\n) {\n  libraryDirectory(library: $library, path: $path) {\n    path\n    entries {\n      name\n      path\n      directory\n      size\n      modifiedAt\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "fa876ee68f31a4829f5ec044eac348c0";

export default node;
