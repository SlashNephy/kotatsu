/**
 * @generated SignedSource<<14ddb960becd3903f3908e51a76f391d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaRecordDestination = "SIMKL" | "%future added value";
export type DramaRecordAppInput = {
  clientId: string;
  clientSecret?: string | null | undefined;
};
export type integrationsDramaRecordAppMutation$variables = {
  destination: DramaRecordDestination;
  input: DramaRecordAppInput;
};
export type integrationsDramaRecordAppMutation$data = {
  readonly updateDramaRecordApp: {
    readonly clientId: string | null | undefined;
    readonly configured: boolean;
    readonly destination: DramaRecordDestination;
  };
};
export type integrationsDramaRecordAppMutation = {
  response: integrationsDramaRecordAppMutation$data;
  variables: integrationsDramaRecordAppMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "destination"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "destination",
        "variableName": "destination"
      },
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "DramaRecordApp",
    "kind": "LinkedField",
    "name": "updateDramaRecordApp",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "destination",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "clientId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "configured",
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
    "name": "integrationsDramaRecordAppMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsDramaRecordAppMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "fecd00b7aed88afc94d90a47f4da42f3",
    "id": null,
    "metadata": {},
    "name": "integrationsDramaRecordAppMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsDramaRecordAppMutation(\n  $destination: DramaRecordDestination!\n  $input: DramaRecordAppInput!\n) {\n  updateDramaRecordApp(destination: $destination, input: $input) {\n    destination\n    clientId\n    configured\n  }\n}\n"
  }
};
})();

(node as any).hash = "c8cdd6b3a36ce948174e0a7342259c2f";

export default node;
