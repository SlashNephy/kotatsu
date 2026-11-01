/**
 * @generated SignedSource<<3d64bfccb1c20752b95be495caf8bd8f>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type timeshiftQuery$variables = Record<PropertyKey, never>;
export type timeshiftQuery$data = {
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly displayName: string;
    };
    readonly id: string;
    readonly logoUrl: string | null | undefined;
    readonly name: string;
    readonly serviceId: number;
  }>;
  readonly timeshiftAvailable: boolean;
  readonly timeshiftChannels: ReadonlyArray<{
    readonly bufferedFrom: string | null | undefined;
    readonly bufferedUntil: string | null | undefined;
    readonly recording: boolean;
    readonly retentionSeconds: number;
    readonly service: {
      readonly channelType: {
        readonly displayName: string;
      };
      readonly id: string;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
  }>;
};
export type timeshiftQuery = {
  response: timeshiftQuery$data;
  variables: timeshiftQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v3 = {
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
},
v4 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "Service",
    "kind": "LinkedField",
    "name": "services",
    "plural": true,
    "selections": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "serviceId",
        "storageKey": null
      },
      (v3/*:: as any*/)
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "timeshiftAvailable",
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "TimeshiftChannel",
    "kind": "LinkedField",
    "name": "timeshiftChannels",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "Service",
        "kind": "LinkedField",
        "name": "service",
        "plural": false,
        "selections": [
          (v0/*:: as any*/),
          (v1/*:: as any*/),
          (v2/*:: as any*/),
          (v3/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "retentionSeconds",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "bufferedFrom",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "bufferedUntil",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "recording",
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
    "name": "timeshiftQuery",
    "selections": (v4/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "timeshiftQuery",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "c6c418816e5eefdb2392af12ffd512d7",
    "id": null,
    "metadata": {},
    "name": "timeshiftQuery",
    "operationKind": "query",
    "text": "query timeshiftQuery {\n  services {\n    id\n    name\n    logoUrl\n    serviceId\n    channelType {\n      displayName\n    }\n  }\n  timeshiftAvailable\n  timeshiftChannels {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        displayName\n      }\n    }\n    retentionSeconds\n    bufferedFrom\n    bufferedUntil\n    recording\n  }\n}\n"
  }
};
})();

(node as any).hash = "aace4434466f70ec8d609edd305d306f";

export default node;
