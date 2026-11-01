/**
 * @generated SignedSource<<832f2439ef422fe5f085f18ecb1c4ddb>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type StreamKind = "AUDIO" | "OTHER" | "PSI" | "UNKNOWN" | "VIDEO" | "%future added value";
export type ChannelInspectorMutation$variables = {
  channel: string;
  channelType: string;
};
export type ChannelInspectorMutation$data = {
  readonly inspectChannel: {
    readonly others: ReadonlyArray<{
      readonly bps: number;
      readonly kind: StreamKind;
      readonly pid: number;
    }>;
    readonly programs: ReadonlyArray<{
      readonly bps: number;
      readonly name: string | null | undefined;
      readonly serviceId: number;
      readonly streams: ReadonlyArray<{
        readonly bps: number;
        readonly drops: number;
        readonly errors: number;
        readonly kind: StreamKind;
        readonly pid: number;
        readonly scrambled: number;
        readonly streamType: number | null | undefined;
      }>;
    }>;
    readonly seconds: number;
    readonly totalBps: number;
    readonly transportStreamId: number | null | undefined;
  };
};
export type ChannelInspectorMutation = {
  response: ChannelInspectorMutation$data;
  variables: ChannelInspectorMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "channel"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "channelType"
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "bps",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "pid",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v5 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "channel",
        "variableName": "channel"
      },
      {
        "kind": "Variable",
        "name": "channelType",
        "variableName": "channelType"
      }
    ],
    "concreteType": "ChannelInspection",
    "kind": "LinkedField",
    "name": "inspectChannel",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "transportStreamId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "seconds",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "totalBps",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "InspectedProgram",
        "kind": "LinkedField",
        "name": "programs",
        "plural": true,
        "selections": [
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
            "kind": "ScalarField",
            "name": "name",
            "storageKey": null
          },
          (v2/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "concreteType": "InspectedStream",
            "kind": "LinkedField",
            "name": "streams",
            "plural": true,
            "selections": [
              (v3/*:: as any*/),
              (v4/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "streamType",
                "storageKey": null
              },
              (v2/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "drops",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "errors",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "scrambled",
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
        "concreteType": "InspectedStream",
        "kind": "LinkedField",
        "name": "others",
        "plural": true,
        "selections": [
          (v3/*:: as any*/),
          (v4/*:: as any*/),
          (v2/*:: as any*/)
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
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "ChannelInspectorMutation",
    "selections": (v5/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "ChannelInspectorMutation",
    "selections": (v5/*:: as any*/)
  },
  "params": {
    "cacheID": "99df3b85f4007d63cc7bd731b1f695be",
    "id": null,
    "metadata": {},
    "name": "ChannelInspectorMutation",
    "operationKind": "mutation",
    "text": "mutation ChannelInspectorMutation(\n  $channelType: String!\n  $channel: String!\n) {\n  inspectChannel(channelType: $channelType, channel: $channel) {\n    transportStreamId\n    seconds\n    totalBps\n    programs {\n      serviceId\n      name\n      bps\n      streams {\n        pid\n        kind\n        streamType\n        bps\n        drops\n        errors\n        scrambled\n      }\n    }\n    others {\n      pid\n      kind\n      bps\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "8aa6d0800f437947ac5a52c3ad037c21";

export default node;
