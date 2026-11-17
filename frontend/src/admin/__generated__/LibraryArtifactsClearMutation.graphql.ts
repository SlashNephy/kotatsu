/**
 * @generated SignedSource<<480245d201a13e2f2784f58ecdef986c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LibraryArtifactsClearMutation$variables = Record<PropertyKey, never>;
export type LibraryArtifactsClearMutation$data = {
  readonly clearLibraryArtifacts: {
    readonly files: number;
    readonly size: number;
  };
};
export type LibraryArtifactsClearMutation = {
  response: LibraryArtifactsClearMutation$data;
  variables: LibraryArtifactsClearMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "LibraryArtifacts",
    "kind": "LinkedField",
    "name": "clearLibraryArtifacts",
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
    "name": "LibraryArtifactsClearMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "LibraryArtifactsClearMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "d903c9ee40fb6b8309d6cce7efb20e1c",
    "id": null,
    "metadata": {},
    "name": "LibraryArtifactsClearMutation",
    "operationKind": "mutation",
    "text": "mutation LibraryArtifactsClearMutation {\n  clearLibraryArtifacts {\n    size\n    files\n  }\n}\n"
  }
};
})();

(node as any).hash = "ee1c9dd292c5061884ca65943a7857f9";

export default node;
