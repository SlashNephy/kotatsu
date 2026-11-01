/**
 * @generated SignedSource<<4b4e4351c16d4457cb66d7ea566da3d8>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ReservationKind = "PROGRAM" | "RULE" | "TIME" | "%future added value";
export type ReservationSettingsDialogQuery$variables = {
  id: string;
};
export type ReservationSettingsDialogQuery$data = {
  readonly node: {
    readonly __typename: "Program";
    readonly id: string;
  } | {
    readonly __typename: "Reservation";
    readonly createdAt: string;
    readonly createdBy: {
      readonly displayName: string;
    } | null | undefined;
    readonly followEventRelay: boolean;
    readonly kind: ReservationKind;
    readonly rules: ReadonlyArray<{
      readonly name: string;
      readonly settings: {
        readonly detectCm: boolean | null | undefined;
        readonly endMarginSeconds: number | null | undefined;
        readonly fileNameTemplate: string | null | undefined;
        readonly joinSegments: boolean | null | undefined;
        readonly poolId: string | null | undefined;
        readonly priority: number | null | undefined;
        readonly startMarginSeconds: number | null | undefined;
      };
    }>;
    readonly settings: {
      readonly detectCm: boolean | null | undefined;
      readonly endMarginSeconds: number | null | undefined;
      readonly fileNameTemplate: string | null | undefined;
      readonly joinSegments: boolean | null | undefined;
      readonly poolId: string | null | undefined;
      readonly priority: number | null | undefined;
      readonly startMarginSeconds: number | null | undefined;
    };
    readonly updatedAt: string;
    readonly updatedBy: {
      readonly displayName: string;
    } | null | undefined;
  } | {
    // This will never be '%other', but we need some
    // value in case none of the concrete values match.
    readonly __typename: "%other";
  } | null | undefined;
  readonly storagePoolNames: ReadonlyArray<string>;
  readonly systemSettings: {
    readonly recordingDefaults: {
      readonly detectCm: boolean;
      readonly joinSegments: boolean;
      readonly poolId: string | null | undefined;
      readonly priority: number;
    };
  };
};
export type ReservationSettingsDialogQuery = {
  response: ReservationSettingsDialogQuery$data;
  variables: ReservationSettingsDialogQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  }
],
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "poolId",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "priority",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "joinSegments",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "detectCm",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "concreteType": "SystemSettings",
  "kind": "LinkedField",
  "name": "systemSettings",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "concreteType": "RecordingSettings",
      "kind": "LinkedField",
      "name": "recordingDefaults",
      "plural": false,
      "selections": [
        (v1/*:: as any*/),
        (v2/*:: as any*/),
        (v3/*:: as any*/),
        (v4/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "storagePoolNames",
  "storageKey": null
},
v7 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "followEventRelay",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "createdAt",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v14 = [
  (v13/*:: as any*/)
],
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "updatedAt",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startMarginSeconds",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endMarginSeconds",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "fileNameTemplate",
  "storageKey": null
},
v20 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingSettingsOverrides",
  "kind": "LinkedField",
  "name": "settings",
  "plural": false,
  "selections": [
    (v1/*:: as any*/),
    (v17/*:: as any*/),
    (v18/*:: as any*/),
    (v19/*:: as any*/),
    (v2/*:: as any*/),
    (v3/*:: as any*/),
    (v4/*:: as any*/)
  ],
  "storageKey": null
},
v21 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingSettingsOverrides",
  "kind": "LinkedField",
  "name": "settings",
  "plural": false,
  "selections": [
    (v1/*:: as any*/),
    (v19/*:: as any*/),
    (v17/*:: as any*/),
    (v18/*:: as any*/),
    (v2/*:: as any*/),
    (v3/*:: as any*/),
    (v4/*:: as any*/)
  ],
  "storageKey": null
},
v22 = [
  (v13/*:: as any*/),
  (v9/*:: as any*/)
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ReservationSettingsDialogQuery",
    "selections": [
      (v5/*:: as any*/),
      (v6/*:: as any*/),
      {
        "alias": null,
        "args": (v7/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v8/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v9/*:: as any*/)
            ],
            "type": "Program",
            "abstractKey": null
          },
          {
            "kind": "InlineFragment",
            "selections": [
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "createdBy",
                "plural": false,
                "selections": (v14/*:: as any*/),
                "storageKey": null
              },
              (v15/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "updatedBy",
                "plural": false,
                "selections": (v14/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "Rule",
                "kind": "LinkedField",
                "name": "rules",
                "plural": true,
                "selections": [
                  (v16/*:: as any*/),
                  (v20/*:: as any*/)
                ],
                "storageKey": null
              },
              (v21/*:: as any*/)
            ],
            "type": "Reservation",
            "abstractKey": null
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
    "name": "ReservationSettingsDialogQuery",
    "selections": [
      (v5/*:: as any*/),
      (v6/*:: as any*/),
      {
        "alias": null,
        "args": (v7/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v8/*:: as any*/),
          (v9/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              (v12/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "createdBy",
                "plural": false,
                "selections": (v22/*:: as any*/),
                "storageKey": null
              },
              (v15/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "updatedBy",
                "plural": false,
                "selections": (v22/*:: as any*/),
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "concreteType": "Rule",
                "kind": "LinkedField",
                "name": "rules",
                "plural": true,
                "selections": [
                  (v16/*:: as any*/),
                  (v20/*:: as any*/),
                  (v9/*:: as any*/)
                ],
                "storageKey": null
              },
              (v21/*:: as any*/)
            ],
            "type": "Reservation",
            "abstractKey": null
          }
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "effd144af5c0c0c9a2ac64b240c8413f",
    "id": null,
    "metadata": {},
    "name": "ReservationSettingsDialogQuery",
    "operationKind": "query",
    "text": "query ReservationSettingsDialogQuery(\n  $id: ID!\n) {\n  systemSettings {\n    recordingDefaults {\n      poolId\n      priority\n      joinSegments\n      detectCm\n    }\n  }\n  storagePoolNames\n  node(id: $id) {\n    __typename\n    ... on Program {\n      id\n    }\n    ... on Reservation {\n      kind\n      followEventRelay\n      createdAt\n      createdBy {\n        displayName\n        id\n      }\n      updatedAt\n      updatedBy {\n        displayName\n        id\n      }\n      rules {\n        name\n        settings {\n          poolId\n          startMarginSeconds\n          endMarginSeconds\n          fileNameTemplate\n          priority\n          joinSegments\n          detectCm\n        }\n        id\n      }\n      settings {\n        poolId\n        fileNameTemplate\n        startMarginSeconds\n        endMarginSeconds\n        priority\n        joinSegments\n        detectCm\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "079bb9d9e3f09b640c5911fcc1802ae6";

export default node;
