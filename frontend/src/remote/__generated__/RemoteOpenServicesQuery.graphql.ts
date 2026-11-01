/**
 * @generated SignedSource<<eb76ff15c4c65643c4eadd0cce99c99d>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RemoteOpenServicesQuery$variables = Record<PropertyKey, never>;
export type RemoteOpenServicesQuery$data = {
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly displayName: string;
    };
    readonly id: string;
    readonly name: string;
  }>;
};
export type RemoteOpenServicesQuery = {
  response: RemoteOpenServicesQuery$data;
  variables: RemoteOpenServicesQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "services",
    "plural": true,
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
        "name": "name",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "ChannelType",
        "kind": "LinkedField",
        "name": "channelType",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "displayName",
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
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "RemoteOpenServicesQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "RemoteOpenServicesQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "8786aded2d487c492c4e276806137f44",
    "id": null,
    "metadata": {},
    "name": "RemoteOpenServicesQuery",
    "operationKind": "query",
    "text": "query RemoteOpenServicesQuery {\n  services {\n    id\n    name\n    channelType {\n      displayName\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "0f306dc78a3f682004b7a51a05e54307";

export default node;
