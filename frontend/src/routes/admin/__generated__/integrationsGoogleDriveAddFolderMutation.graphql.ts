/**
 * @generated SignedSource<<f17ae18ab63b7071ff19322ac074cb37>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsGoogleDriveAddFolderMutation$variables = {
  id: string;
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
    "cacheID": "b4a87ddff75b0be4d318d8fa4df38161",
    "id": null,
    "metadata": {},
    "name": "integrationsGoogleDriveAddFolderMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsGoogleDriveAddFolderMutation(\n  $id: String!\n) {\n  addGoogleDriveFolder(id: $id) {\n    folders {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "29934b132b323b801f1845414f14b979";

export default node;
