/**
 * @generated SignedSource<<50eb8a27dd5ecb065d0847f553d4df73>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type librarySearchQuery$variables = {
  library: string;
  path: string;
  query: string;
};
export type librarySearchQuery$data = {
  readonly librarySearch: {
    readonly hits: ReadonlyArray<{
      readonly entry: {
        readonly modifiedAt: string;
        readonly name: string;
        readonly path: string;
        readonly size: number;
      };
      readonly folderNames: ReadonlyArray<string>;
    }>;
    readonly truncated: boolean;
  };
};
export type librarySearchQuery = {
  response: librarySearchQuery$data;
  variables: librarySearchQuery$variables;
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
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "query"
  }
],
v1 = [
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
      },
      {
        "kind": "Variable",
        "name": "query",
        "variableName": "query"
      }
    ],
    "concreteType": "LibrarySearchResult",
    "kind": "LinkedField",
    "name": "librarySearch",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "truncated",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "LibrarySearchHit",
        "kind": "LinkedField",
        "name": "hits",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "folderNames",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "LibraryEntry",
            "kind": "LinkedField",
            "name": "entry",
            "plural": false,
            "selections": [
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
                "name": "path",
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
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "librarySearchQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "librarySearchQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "6fad55cd8d82f9b0e6fed66b65b9907f",
    "id": null,
    "metadata": {},
    "name": "librarySearchQuery",
    "operationKind": "query",
    "text": "query librarySearchQuery(\n  $library: String!\n  $path: String!\n  $query: String!\n) {\n  librarySearch(library: $library, path: $path, query: $query) {\n    truncated\n    hits {\n      folderNames\n      entry {\n        name\n        path\n        size\n        modifiedAt\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "fd5a1659d4c8da9e7c4b950712850e9c";

export default node;
