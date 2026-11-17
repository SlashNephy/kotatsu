/**
 * @generated SignedSource<<659b94154e574f5dfda2e41715c30a93>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LibraryKind = "FOLDER" | "GOOGLE_DRIVE" | "POOL" | "%future added value";
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
    readonly library: {
      readonly kind: LibraryKind;
    };
    readonly names: ReadonlyArray<string>;
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
      {
        "alias": null,
        "args": null,
        "concreteType": "Library",
        "kind": "LinkedField",
        "name": "library",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "kind",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v1/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "names",
        "storageKey": null
      },
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
    "cacheID": "b910173e13665fa05dde01b2771dbbaa",
    "id": null,
    "metadata": {},
    "name": "libraryDirectoryQuery",
    "operationKind": "query",
    "text": "query libraryDirectoryQuery(\n  $library: String!\n  $path: String!\n) {\n  libraryDirectory(library: $library, path: $path) {\n    library {\n      kind\n    }\n    path\n    names\n    entries {\n      name\n      path\n      directory\n      size\n      modifiedAt\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "8aebaffb5ccc2a1977c189484d151991";

export default node;
