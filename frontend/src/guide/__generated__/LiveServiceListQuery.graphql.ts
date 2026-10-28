/**
 * @generated SignedSource<<67ee20be8bdd60deacf0cff7c2751d02>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LiveServiceListQuery$variables = {
  from: string;
  to: string;
};
export type LiveServiceListQuery$data = {
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
};
export type LiveServiceListQuery = {
  response: LiveServiceListQuery$data;
  variables: LiveServiceListQuery$variables;
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
v3 = [
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
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "service",
  "plural": false,
  "selections": [
    (v4/*:: as any*/),
    (v5/*:: as any*/),
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
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startAt",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endAt",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "sharedFrom",
  "plural": false,
  "selections": [
    (v4/*:: as any*/)
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "LiveServiceListQuery",
    "selections": [
      (v2/*:: as any*/),
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
          (v6/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "programs",
            "plural": true,
            "selections": [
              (v5/*:: as any*/),
              (v7/*:: as any*/),
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
    "name": "LiveServiceListQuery",
    "selections": [
      (v2/*:: as any*/),
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
          (v6/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Program",
            "kind": "LinkedField",
            "name": "programs",
            "plural": true,
            "selections": [
              (v5/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              (v9/*:: as any*/),
              (v4/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "6128bd274069167a3ebf7676016ab5ef",
    "id": null,
    "metadata": {},
    "name": "LiveServiceListQuery",
    "operationKind": "query",
    "text": "query LiveServiceListQuery(\n  $from: Time!\n  $to: Time!\n) {\n  channelTypes {\n    type\n    displayName\n  }\n  guide(from: $from, to: $to) {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        type\n      }\n    }\n    programs {\n      name\n      startAt\n      endAt\n      sharedFrom {\n        id\n      }\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "1b66c430a85ca8206406e3ccb1b51b21";

export default node;
