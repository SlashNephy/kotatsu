/**
 * @generated SignedSource<<ec6500c1c8f6750307b3793a349793f1>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type VideoCodec = "H264" | "HEVC" | "%future added value";
export type VideoFormat = "HD" | "SHV" | "UHD" | "%future added value";
export type ServiceIdLiveQuery$variables = {
  from: string;
  id: string;
  to: string;
};
export type ServiceIdLiveQuery$data = {
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly audios: ReadonlyArray<{
        readonly componentType: number;
        readonly languages: ReadonlyArray<string>;
      }>;
      readonly endAt: string;
      readonly endUndecided: boolean;
      readonly name: string;
      readonly startAt: string;
    }>;
  }>;
  readonly node: {
    readonly __typename: "Service";
    readonly jikkyo: {
      readonly id: string;
      readonly name: string;
    } | null | undefined;
    readonly logoUrl: string | null | undefined;
    readonly name: string;
    readonly streamUrl: string;
    readonly timeshiftPrograms: ReadonlyArray<{
      readonly startAt: string;
    }>;
    readonly videoFormat: VideoFormat;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly services: ReadonlyArray<{
    readonly channelType: {
      readonly type: string;
    };
    readonly id: string;
    readonly name: string;
    readonly networkId: number;
    readonly serviceId: number;
  }>;
  readonly transcoding: {
    readonly available: boolean;
    readonly presets: ReadonlyArray<{
      readonly codec: VideoCodec;
      readonly name: string;
      readonly passthrough: boolean;
    }>;
  };
};
export type ServiceIdLiveQuery = {
  response: ServiceIdLiveQuery$data;
  variables: ServiceIdLiveQuery$variables;
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
  "name": "id"
},
v2 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "to"
},
v3 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
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
  "kind": "ScalarField",
  "name": "id",
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
  "kind": "InlineFragment",
  "selections": [
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
      "kind": "ScalarField",
      "name": "streamUrl",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "videoFormat",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "JikkyoChannel",
      "kind": "LinkedField",
      "name": "jikkyo",
      "plural": false,
      "selections": [
        (v6/*:: as any*/),
        (v5/*:: as any*/)
      ],
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "TimeshiftProgram",
      "kind": "LinkedField",
      "name": "timeshiftPrograms",
      "plural": true,
      "selections": [
        (v7/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "type": "Service",
  "abstractKey": null
},
v9 = [
  {
    "kind": "Variable",
    "name": "from",
    "variableName": "from"
  },
  {
    "items": [
      {
        "kind": "Variable",
        "name": "services.0",
        "variableName": "id"
      }
    ],
    "kind": "ListValue",
    "name": "services"
  },
  {
    "kind": "Variable",
    "name": "to",
    "variableName": "to"
  }
],
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
  "kind": "ScalarField",
  "name": "endUndecided",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "concreteType": "ProgramAudio",
  "kind": "LinkedField",
  "name": "audios",
  "plural": true,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "componentType",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "languages",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "concreteType": "Transcoding",
  "kind": "LinkedField",
  "name": "transcoding",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "available",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "concreteType": "StreamPreset",
      "kind": "LinkedField",
      "name": "presets",
      "plural": true,
      "selections": [
        (v5/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "codec",
          "storageKey": null
        },
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "passthrough",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "concreteType": "Service",
  "kind": "LinkedField",
  "name": "services",
  "plural": true,
  "selections": [
    (v6/*:: as any*/),
    (v5/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "networkId",
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
          "name": "type",
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "ServiceIdLiveQuery",
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v4/*:: as any*/),
          (v8/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v9/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
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
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v13/*:: as any*/),
      (v14/*:: as any*/)
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/),
      (v2/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "ServiceIdLiveQuery",
    "selections": [
      {
        "alias": null,
        "args": (v3/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v4/*:: as any*/),
          (v8/*:: as any*/),
          (v6/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v9/*:: as any*/),
        "concreteType": "GuideRow",
        "kind": "LinkedField",
        "name": "guide",
        "plural": true,
        "selections": [
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
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              (v6/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v13/*:: as any*/),
      (v14/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "cbd7c94fed4d237acb0a0ab7c32a3870",
    "id": null,
    "metadata": {},
    "name": "ServiceIdLiveQuery",
    "operationKind": "query",
    "text": "query ServiceIdLiveQuery(\n  $id: ID!\n  $from: Time!\n  $to: Time!\n) {\n  node(id: $id) {\n    __typename\n    ... on Service {\n      name\n      logoUrl\n      streamUrl\n      videoFormat\n      jikkyo {\n        id\n        name\n      }\n      timeshiftPrograms {\n        startAt\n      }\n    }\n    id\n  }\n  guide(from: $from, to: $to, services: [$id]) {\n    programs {\n      name\n      startAt\n      endAt\n      endUndecided\n      audios {\n        componentType\n        languages\n      }\n      id\n    }\n  }\n  transcoding {\n    available\n    presets {\n      name\n      codec\n      passthrough\n    }\n  }\n  services {\n    id\n    name\n    networkId\n    serviceId\n    channelType {\n      type\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "2b05458387d0e5d334a6fb4657d5d6a5";

export default node;
