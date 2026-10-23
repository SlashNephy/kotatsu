/**
 * @generated SignedSource<<050613c604b319d319f926f509667c81>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type channelServiceIdGuideQuery$variables = {
  from: string;
  services?: ReadonlyArray<string> | null | undefined;
  to: string;
};
export type channelServiceIdGuideQuery$data = {
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly endAt: string;
      readonly endUndecided: boolean;
      readonly genres: ReadonlyArray<{
        readonly lv1: number;
        readonly lv2: number;
      }>;
      readonly id: string;
      readonly name: string;
      readonly reservation: {
        readonly enabled: boolean;
        readonly id: string;
      } | null | undefined;
      readonly startAt: string;
      readonly watchReserved: boolean;
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
export type channelServiceIdGuideQuery = {
  response: channelServiceIdGuideQuery$data;
  variables: channelServiceIdGuideQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "from"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "services"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "to"
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v5 = [
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
        "name": "services",
        "variableName": "services"
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
          (v3/*:: as any*/),
          (v4/*:: as any*/),
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
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "type",
                "storageKey": null
              }
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
          (v3/*:: as any*/),
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
          (v4/*:: as any*/),
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
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "lv2",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "Reservation",
            "kind": "LinkedField",
            "name": "reservation",
            "plural": false,
            "selections": [
              (v3/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "enabled",
                "storageKey": null
              }
            ],
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "watchReserved",
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
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "channelServiceIdGuideQuery",
    "selections": (v5/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v2/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "channelServiceIdGuideQuery",
    "selections": (v5/*:: as any*/)
  },
  "params": {
    "cacheID": "87be95dc4d5be9c7dc2645e2acdb74da",
    "id": null,
    "metadata": {},
    "name": "channelServiceIdGuideQuery",
    "operationKind": "query",
    "text": "query channelServiceIdGuideQuery(\n  $from: Time!\n  $to: Time!\n  $services: [ID!]\n) {\n  guide(from: $from, to: $to, services: $services) {\n    service {\n      id\n      name\n      logoUrl\n      channelType {\n        type\n      }\n    }\n    programs {\n      id\n      startAt\n      endAt\n      endUndecided\n      name\n      genres {\n        lv1\n        lv2\n      }\n      reservation {\n        id\n        enabled\n      }\n      watchReserved\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "96837e63215d8d8b71acf8b9bbe36c24";

export default node;
