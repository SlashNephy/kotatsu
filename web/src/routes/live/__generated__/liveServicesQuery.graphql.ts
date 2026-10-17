/**
 * @generated SignedSource<<f3a98d392cb0ebd7f4c7e63192a3ad8c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type liveServicesQuery$variables = {
  from: string;
  to: string;
};
export type liveServicesQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly type: string;
  }>;
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly endAt: string;
      readonly name: string;
      readonly sharedFrom: {
        readonly id: string;
      } | null | undefined;
      readonly startAt: string;
    }>;
    readonly service: {
      readonly channelType: {
        readonly type: string;
      };
      readonly id: string;
      readonly logoUrl: string | null | undefined;
      readonly name: string;
    };
  }>;
  readonly pinnedServices: ReadonlyArray<{
    readonly id: string;
  }>;
};
export type liveServicesQuery = {
  response: liveServicesQuery$data;
  variables: liveServicesQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "from"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "to"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "type",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "concreteType": "ChannelType",
  "kind": "LinkedField",
  "name": "channelTypes",
  "plural": true,
  "selections": [
    (v1/*:: as any*/),
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
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v4 = [
  (v3/*:: as any*/)
],
v5 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "pinnedServices",
  "plural": true,
  "selections": (v4/*:: as any*/),
  "storageKey": null
},
v6 = [
  {
    "kind": "Variable",
    "name": "from",
    "variableName": "from"
  },
  {
    "kind": "Variable",
    "name": "to",
    "variableName": "to"
  }
],
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v3/*:: as any*/),
    (v7/*:: as any*/),
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
      "concreteType": "ChannelType",
      "kind": "LinkedField",
      "name": "channelType",
      "plural": false,
      "selections": [
        (v1/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "sharedFrom",
  "plural": false,
  "selections": (v4/*:: as any*/),
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "liveServicesQuery",
    "selections": [
      (v2/*:: as any*/),
      (v5/*:: as any*/),
      {
        "alias": null,
        "args": (v6/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
          (v8/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "programs",
            "plural": true,
            "selections": [
              (v7/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/)
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
    "name": "liveServicesQuery",
    "selections": [
      (v2/*:: as any*/),
      (v5/*:: as any*/),
      {
        "alias": null,
        "args": (v6/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
          (v8/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "programs",
            "plural": true,
            "selections": [
              (v7/*:: as any*/),
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v3/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "714607be4282cb044ed59a7063899a97",
    "id": null,
    "metadata": {},
    "name": "liveServicesQuery",
    "operationKind": "query",
    "text": "query liveServicesQuery(\n  $from: Time!\n  $to: Time!\n) {\n  channelTypes {\n    type\n    displayName\n  }\n  pinnedServices {\n    id\n  }\n  guide(from: $from, to: $to) {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        type\n      }\n    }\n    programs {\n      name\n      startAt\n      endAt\n      sharedFrom {\n        id\n      }\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "05538ae0ef8bc33eaf299ce708cf03d9";

export default node;
