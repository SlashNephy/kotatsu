/**
 * @generated SignedSource<<4363034d95da33bd87c833fe3b95afd9>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type VideoCodec = "H264" | "HEVC" | "%future added value";
export type ServiceIdLiveQuery$variables = {
  from: string;
  id: string;
  to: string;
};
export type ServiceIdLiveQuery$data = {
  readonly guide: ReadonlyArray<{
    readonly programs: ReadonlyArray<{
      readonly name: string;
    }>;
  }>;
  readonly node: {
    readonly __typename: "Service";
    readonly jikkyo: {
      readonly id: string;
      readonly name: string;
    } | null | undefined;
    readonly name: string;
    readonly streamUrl: string;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly transcoding: {
    readonly available: boolean;
    readonly presets: ReadonlyArray<{
      readonly codec: VideoCodec;
      readonly name: string;
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
  "kind": "InlineFragment",
  "selections": [
    (v5/*:: as any*/),
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
      "concreteType": "JikkyoChannel",
      "kind": "LinkedField",
      "name": "jikkyo",
      "plural": false,
      "selections": [
        (v6/*:: as any*/),
        (v5/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "type": "Service",
  "abstractKey": null
},
v8 = [
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
v9 = {
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
          (v7/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v8/*:: as any*/),
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
              (v5/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v9/*:: as any*/)
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
          (v7/*:: as any*/),
          (v6/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": (v8/*:: as any*/),
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
              (v6/*:: as any*/)
            ],
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      (v9/*:: as any*/)
    ]
  },
  "params": {
    "cacheID": "cd36ca74c7dd188af87dcb113bcdcf38",
    "id": null,
    "metadata": {},
    "name": "ServiceIdLiveQuery",
    "operationKind": "query",
    "text": "query ServiceIdLiveQuery(\n  $id: ID!\n  $from: Time!\n  $to: Time!\n) {\n  node(id: $id) {\n    __typename\n    ... on Service {\n      name\n      streamUrl\n      jikkyo {\n        id\n        name\n      }\n    }\n    id\n  }\n  guide(from: $from, to: $to, services: [$id]) {\n    programs {\n      name\n      id\n    }\n  }\n  transcoding {\n    available\n    presets {\n      name\n      codec\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "048b02fb4b9bec33de5a64662e70212b";

export default node;
