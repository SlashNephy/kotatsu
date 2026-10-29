/**
 * @generated SignedSource<<fa4468048a55dba0fdc28c78efb35c33>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type VideoFormat = "HD" | "SHV" | "UHD" | "%future added value";
export type ServiceIdPlayTimeshiftQuery$variables = {
  serviceId: string;
  startAt: string;
};
export type ServiceIdPlayTimeshiftQuery$data = {
  readonly timeshiftProgram: {
    readonly endAt: string | null | undefined;
    readonly gaps: ReadonlyArray<{
      readonly from: string;
      readonly to: string;
    }>;
    readonly name: string;
    readonly playableFrom: string;
    readonly service: {
      readonly logoUrl: string | null | undefined;
      readonly name: string;
      readonly videoFormat: VideoFormat;
    };
    readonly startAt: string;
    readonly streamUrl: string;
  };
};
export type ServiceIdPlayTimeshiftQuery = {
  response: ServiceIdPlayTimeshiftQuery$data;
  variables: ServiceIdPlayTimeshiftQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "serviceId"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "startAt"
  }
],
v1 = [
  {
    "kind": "Variable",
    "name": "serviceId",
    "variableName": "serviceId"
  },
  {
    "kind": "Variable",
    "name": "startAt",
    "variableName": "startAt"
  }
],
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "playableFrom",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingGap",
  "kind": "LinkedField",
  "name": "gaps",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "from",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "to",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "streamUrl",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "videoFormat",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ServiceIdPlayTimeshiftQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "TimeshiftProgram",
        "kind": "LinkedField",
        "name": "timeshiftProgram",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          (v7/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Service",
            "kind": "LinkedField",
            "name": "service",
            "plural": false,
            "selections": [
              (v2/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ServiceIdPlayTimeshiftQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": "TimeshiftProgram",
        "kind": "LinkedField",
        "name": "timeshiftProgram",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          (v5/*:: as any*/),
          (v6/*:: as any*/),
          (v7/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Service",
            "kind": "LinkedField",
            "name": "service",
            "plural": false,
            "selections": [
              (v2/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
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
    ]
  },
  "params": {
    "cacheID": "4bd0cbe40142b1287d667bbc83abd4ed",
    "id": null,
    "metadata": {},
    "name": "ServiceIdPlayTimeshiftQuery",
    "operationKind": "query",
    "text": "query ServiceIdPlayTimeshiftQuery(\n  $serviceId: ID!\n  $startAt: Time!\n) {\n  timeshiftProgram(serviceId: $serviceId, startAt: $startAt) {\n    name\n    startAt\n    endAt\n    playableFrom\n    gaps {\n      from\n      to\n    }\n    streamUrl\n    service {\n      name\n      logoUrl\n      videoFormat\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "6e5bb48e0b54d717504c0937566e54ca";

export default node;
