/**
 * @generated SignedSource<<bbb8fbd5331596c6100bcca590f0adfd>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type OrphanFileInput = {
  path: string;
  poolId: string;
  sidecar: boolean;
};
export type OrphanFilesDeleteMutation$variables = {
  files: ReadonlyArray<OrphanFileInput>;
};
export type OrphanFilesDeleteMutation$data = {
  readonly deleteOrphanFiles: {
    readonly deleted: number;
    readonly skipped: number;
  };
};
export type OrphanFilesDeleteMutation = {
  response: OrphanFilesDeleteMutation$data;
  variables: OrphanFilesDeleteMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "files"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "files",
        "variableName": "files"
      }
    ],
    "concreteType": "DeleteOrphanFilesPayload",
    "kind": "LinkedField",
    "name": "deleteOrphanFiles",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "deleted",
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
    "name": "OrphanFilesDeleteMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "OrphanFilesDeleteMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "beab0fca61eae7a7983154dbc54b7f22",
    "id": null,
    "metadata": {},
    "name": "OrphanFilesDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation OrphanFilesDeleteMutation(\n  $files: [OrphanFileInput!]!\n) {\n  deleteOrphanFiles(files: $files) {\n    deleted\n    skipped\n  }\n}\n"
  }
};
})();

(node as any).hash = "01aa7d10700924d0713e64e10a33dd20";

export default node;
