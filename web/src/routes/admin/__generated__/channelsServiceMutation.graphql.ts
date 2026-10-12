/**
 * @generated SignedSource<<4f754bd312c11761a8ccb038405862f3>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ServiceInput = {
  hidden?: boolean | null | undefined;
  resetSortOrder?: boolean;
  sortOrder?: number | null | undefined;
};
export type channelsServiceMutation$variables = {
  id: string;
  input: ServiceInput;
};
export type channelsServiceMutation$data = {
  readonly updateService: {
    readonly hidden: boolean;
    readonly id: string;
    readonly sortOrder: number | null | undefined;
  };
};
export type channelsServiceMutation = {
  response: channelsServiceMutation$data;
  variables: channelsServiceMutation$variables;
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
    "name": "input"
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
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "updateService",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "hidden",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "sortOrder",
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
    "name": "channelsServiceMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "channelsServiceMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "e444952fe65e0470aa907a1fb911c9fc",
    "id": null,
    "metadata": {},
    "name": "channelsServiceMutation",
    "operationKind": "mutation",
    "text": "mutation channelsServiceMutation(\n  $id: ID!\n  $input: ServiceInput!\n) {\n  updateService(id: $id, input: $input) {\n    id\n    hidden\n    sortOrder\n  }\n}\n"
  }
};
})();

(node as any).hash = "5b6ff24668fce90e5be1fd769023006a";

export default node;
