/**
 * @generated SignedSource<<f658932b8b4dc92b712f43cfce5ea927>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsGoogleDriveAddFolderMutation$variables = {
  id: string;
  libraryName: string;
};
export type integrationsGoogleDriveAddFolderMutation$data = {
  readonly addGoogleDriveFolder: {
    readonly folders: ReadonlyArray<{
      readonly id: string;
    }>;
  };
};
export type integrationsGoogleDriveAddFolderMutation = {
  response: integrationsGoogleDriveAddFolderMutation$data;
  variables: integrationsGoogleDriveAddFolderMutation$variables;
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
    "name": "addGoogleDriveFolder",
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
    "name": "integrationsGoogleDriveAddFolderMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsGoogleDriveAddFolderMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "75f856a6e8988f6a5458110fe82d54c6",
    "id": null,
    "metadata": {},
    "name": "integrationsGoogleDriveAddFolderMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsGoogleDriveAddFolderMutation(\n  $id: String!\n  $libraryName: String!\n) {\n  addGoogleDriveFolder(id: $id, libraryName: $libraryName) {\n    folders {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "95446f82052e05930d06042f1c297d6c";

export default node;
