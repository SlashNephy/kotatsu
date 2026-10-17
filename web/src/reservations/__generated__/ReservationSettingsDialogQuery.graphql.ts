/**
 * @generated SignedSource<<fa0cd769f558d93cb3b89382902eebf9>>
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
        readonly endMarginSeconds: number | null | undefined;
        readonly fileNameTemplate: string | null | undefined;
        readonly joinSegments: boolean | null | undefined;
        readonly poolId: string | null | undefined;
        readonly priority: number | null | undefined;
        readonly startMarginSeconds: number | null | undefined;
      };
    }>;
    readonly settings: {
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
        (v3/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "storagePoolNames",
  "storageKey": null
},
v6 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "followEventRelay",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "createdAt",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v13 = [
  (v12/*:: as any*/)
],
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "updatedAt",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startMarginSeconds",
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endMarginSeconds",
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "fileNameTemplate",
  "storageKey": null
},
v19 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingSettingsOverrides",
  "kind": "LinkedField",
  "name": "settings",
  "plural": false,
  "selections": [
    (v1/*:: as any*/),
    (v16/*:: as any*/),
    (v17/*:: as any*/),
    (v18/*:: as any*/),
    (v2/*:: as any*/),
    (v3/*:: as any*/)
  ],
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
    (v18/*:: as any*/),
    (v16/*:: as any*/),
    (v17/*:: as any*/),
    (v2/*:: as any*/),
    (v3/*:: as any*/)
  ],
  "storageKey": null
},
v21 = [
  (v12/*:: as any*/),
  (v8/*:: as any*/)
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ReservationSettingsDialogQuery",
    "selections": [
      (v4/*:: as any*/),
      (v5/*:: as any*/),
      {
        "alias": null,
        "args": (v6/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v7/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v8/*:: as any*/)
            ],
            "type": "Program",
            "abstractKey": null
          },
          {
            "kind": "InlineFragment",
            "selections": [
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "createdBy",
                "plural": false,
                "selections": (v13/*:: as any*/),
                "storageKey": null
              },
              (v14/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "updatedBy",
                "plural": false,
                "selections": (v13/*:: as any*/),
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
                  (v15/*:: as any*/),
                  (v19/*:: as any*/)
                ],
                "storageKey": null
              },
              (v20/*:: as any*/)
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
      (v4/*:: as any*/),
      (v5/*:: as any*/),
      {
        "alias": null,
        "args": (v6/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v7/*:: as any*/),
          (v8/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v9/*:: as any*/),
              (v10/*:: as any*/),
              (v11/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "createdBy",
                "plural": false,
                "selections": (v21/*:: as any*/),
                "storageKey": null
              },
              (v14/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "updatedBy",
                "plural": false,
                "selections": (v21/*:: as any*/),
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
                  (v15/*:: as any*/),
                  (v19/*:: as any*/),
                  (v8/*:: as any*/)
                ],
                "storageKey": null
              },
              (v20/*:: as any*/)
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
    "cacheID": "917dcfa1a7f8eab8954fd4b3e692db4b",
    "id": null,
    "metadata": {},
    "name": "ReservationSettingsDialogQuery",
    "operationKind": "query",
    "text": "query ReservationSettingsDialogQuery(\n  $id: ID!\n) {\n  systemSettings {\n    recordingDefaults {\n      poolId\n      priority\n      joinSegments\n    }\n  }\n  storagePoolNames\n  node(id: $id) {\n    __typename\n    ... on Program {\n      id\n    }\n    ... on Reservation {\n      kind\n      followEventRelay\n      createdAt\n      createdBy {\n        displayName\n        id\n      }\n      updatedAt\n      updatedBy {\n        displayName\n        id\n      }\n      rules {\n        name\n        settings {\n          poolId\n          startMarginSeconds\n          endMarginSeconds\n          fileNameTemplate\n          priority\n          joinSegments\n        }\n        id\n      }\n      settings {\n        poolId\n        fileNameTemplate\n        startMarginSeconds\n        endMarginSeconds\n        priority\n        joinSegments\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "2c8f3a42344bc436dedf38cf70056548";

export default node;
