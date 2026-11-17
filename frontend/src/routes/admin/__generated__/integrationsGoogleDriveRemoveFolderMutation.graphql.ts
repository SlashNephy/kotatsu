/**
 * @generated SignedSource<<f85ae667a991e563981c5f679e822fbb>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type integrationsGoogleDriveRemoveFolderMutation$variables = {
  id: string;
};
export type integrationsGoogleDriveRemoveFolderMutation$data = {
  readonly removeGoogleDriveFolder: {
    readonly folders: ReadonlyArray<{
      readonly id: string;
    }>;
  };
};
export type integrationsGoogleDriveRemoveFolderMutation = {
  response: integrationsGoogleDriveRemoveFolderMutation$data;
  variables: integrationsGoogleDriveRemoveFolderMutation$variables;
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
    "name": "removeGoogleDriveFolder",
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
    "name": "integrationsGoogleDriveRemoveFolderMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsGoogleDriveRemoveFolderMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "5b237be27ee8a6675af9d636d6a5705a",
    "id": null,
    "metadata": {},
    "name": "integrationsGoogleDriveRemoveFolderMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsGoogleDriveRemoveFolderMutation(\n  $id: String!\n) {\n  removeGoogleDriveFolder(id: $id) {\n    folders {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "0d9aecec3059c1fcc7ad17ff44486808";

export default node;
