/**
 * @generated SignedSource<<4e2d0bb97041240334d4fef58190b242>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsGoogleDriveRenameFolderMutation$variables = {
  id: string;
  libraryName: string;
};
export type integrationsGoogleDriveRenameFolderMutation$data = {
  readonly renameGoogleDriveFolder: {
    readonly folders: ReadonlyArray<{
      readonly id: string;
    }>;
  };
};
export type integrationsGoogleDriveRenameFolderMutation = {
  response: integrationsGoogleDriveRenameFolderMutation$data;
  variables: integrationsGoogleDriveRenameFolderMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "libraryName"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      },
      {
        "kind": "Variable",
        "name": "libraryName",
        "variableName": "libraryName"
      }
    ],
    "concreteType": "GoogleIntegration",
    "kind": "LinkedField",
    "name": "renameGoogleDriveFolder",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "GoogleDriveFolder",
        "kind": "LinkedField",
        "name": "folders",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "id",
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
    "name": "integrationsGoogleDriveRenameFolderMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsGoogleDriveRenameFolderMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "6ea088ff10b63c6f4c88362b99c07916",
    "id": null,
    "metadata": {},
    "name": "integrationsGoogleDriveRenameFolderMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsGoogleDriveRenameFolderMutation(\n  $id: String!\n  $libraryName: String!\n) {\n  renameGoogleDriveFolder(id: $id, libraryName: $libraryName) {\n    folders {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "4f8df2004fde81247426d49d405abba5";

export default node;
