/**
 * @generated SignedSource<<7e425b61f0b035786cce244530402f1f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LibraryArtifactsQuery$variables = Record<PropertyKey, never>;
export type LibraryArtifactsQuery$data = {
  readonly libraryArtifacts: {
    readonly files: number;
    readonly size: number;
  };
};
export type LibraryArtifactsQuery = {
  response: LibraryArtifactsQuery$data;
  variables: LibraryArtifactsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "LibraryArtifacts",
    "kind": "LinkedField",
    "name": "libraryArtifacts",
    "plural": false,
    "selections": [
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
        "name": "files",
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
    "name": "LibraryArtifactsQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "LibraryArtifactsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "b326e4488e5150bc8074f35b9ca7024c",
    "id": null,
    "metadata": {},
    "name": "LibraryArtifactsQuery",
    "operationKind": "query",
    "text": "query LibraryArtifactsQuery {\n  libraryArtifacts {\n    size\n    files\n  }\n}\n"
  }
};
})();

(node as any).hash = "d7be2bc002dac52add7515e37ce77a4f";

export default node;
