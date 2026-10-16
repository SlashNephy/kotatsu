/**
 * @generated SignedSource<<57a41876720553e4d6ac09309647ba91>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type TimeReservationDialogQuery$variables = Record<PropertyKey, never>;
export type TimeReservationDialogQuery$data = {
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly displayName: string;
    };
    readonly id: string;
    readonly logoUrl: string | null | undefined;
    readonly name: string;
    readonly serviceId: number;
  }>;
};
export type TimeReservationDialogQuery = {
  response: TimeReservationDialogQuery$data;
  variables: TimeReservationDialogQuery$variables;
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
        "kind": "ScalarField",
        "name": "logoUrl",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "serviceId",
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
    "name": "TimeReservationDialogQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "TimeReservationDialogQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "c38be86b626941cf255805493f009176",
    "id": null,
    "metadata": {},
    "name": "TimeReservationDialogQuery",
    "operationKind": "query",
    "text": "query TimeReservationDialogQuery {\n  services {\n    id\n    name\n    logoUrl\n    serviceId\n    channelType {\n      displayName\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "2db5ccec907880fb3d0745b3edb482c5";

export default node;
