/**
 * @generated SignedSource<<45951803500018e1d3c57b12aad2391c>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type guideQuery$variables = {
  from: string;
  to: string;
};
export type guideQuery$data = {
  readonly channelTypes: ReadonlyArray<{
    readonly displayName: string;
    readonly type: string;
  }>;
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly endAt: string;
      readonly endUndecided: boolean;
      readonly genres: ReadonlyArray<{
        readonly lv1: number;
      }>;
      readonly id: string;
      readonly name: string;
      readonly startAt: string;
    }>;
    readonly service: {
      readonly channelType: {
        readonly type: string;
      };
      readonly id: string;
      readonly name: string;
    };
  }>;
};
export type guideQuery = {
  response: guideQuery$data;
  variables: guideQuery$variables;
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
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v4 = [
  {
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
  {
    "alias": null,
    "args": [
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
    "concreteType": "GuideRow",
    "kind": "LinkedField",
    "name": "guide",
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
          (v2/*:: as any*/),
          (v3/*:: as any*/),
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
      {
        "alias": null,
        "args": null,
        "concreteType": "Program",
        "kind": "LinkedField",
        "name": "programs",
        "plural": true,
        "selections": [
          (v2/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "startAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "endAt",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "endUndecided",
            "storageKey": null
          },
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "Genre",
            "kind": "LinkedField",
            "name": "genres",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "lv1",
                "storageKey": null
              }
            ],
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
    "name": "guideQuery",
    "selections": (v4/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "guideQuery",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "1a8b155c6cce68addf856861efadc370",
    "id": null,
    "metadata": {},
    "name": "guideQuery",
    "operationKind": "query",
    "text": "query guideQuery(\n  $from: Time!\n  $to: Time!\n) {\n  channelTypes {\n    type\n    displayName\n  }\n  guide(from: $from, to: $to) {\n    service {\n      id\n      name\n      channelType {\n        type\n      }\n    }\n    programs {\n      id\n      startAt\n      endAt\n      endUndecided\n      name\n      genres {\n        lv1\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "7911f3256aabf4ed005bfd462275ce59";

export default node;
