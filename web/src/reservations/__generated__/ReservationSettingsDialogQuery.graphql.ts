/**
 * @generated SignedSource<<ad36f2e1265340f4a17a0b6da4f7cb7b>>
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
  readonly systemSettings: {
    readonly recordingDefaults: {
      readonly joinSegments: boolean;
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
  "name": "priority",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "joinSegments",
  "storageKey": null
},
v3 = {
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
        (v2/*:: as any*/)
      ],
      "storageKey": null
    }
  ],
  "storageKey": null
},
v4 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v6 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "followEventRelay",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "createdAt",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v10 = [
  (v9/*:: as any*/)
],
v11 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "updatedAt",
  "storageKey": null
},
v12 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startMarginSeconds",
  "storageKey": null
},
v14 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endMarginSeconds",
  "storageKey": null
},
v15 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "fileNameTemplate",
  "storageKey": null
},
v16 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingSettingsOverrides",
  "kind": "LinkedField",
  "name": "settings",
  "plural": false,
  "selections": [
    (v13/*:: as any*/),
    (v14/*:: as any*/),
    (v15/*:: as any*/),
    (v1/*:: as any*/),
    (v2/*:: as any*/)
  ],
  "storageKey": null
},
v17 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingSettingsOverrides",
  "kind": "LinkedField",
  "name": "settings",
  "plural": false,
  "selections": [
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "poolId",
      "storageKey": null
    },
    (v15/*:: as any*/),
    (v13/*:: as any*/),
    (v14/*:: as any*/),
    (v1/*:: as any*/),
    (v2/*:: as any*/)
  ],
  "storageKey": null
},
v18 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v19 = [
  (v9/*:: as any*/),
  (v18/*:: as any*/)
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ReservationSettingsDialogQuery",
    "selections": [
      (v3/*:: as any*/),
      {
        "alias": null,
        "args": (v4/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v5/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "createdBy",
                "plural": false,
                "selections": (v10/*:: as any*/),
                "storageKey": null
              },
              (v11/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "updatedBy",
                "plural": false,
                "selections": (v10/*:: as any*/),
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
                  (v12/*:: as any*/),
                  (v16/*:: as any*/)
                ],
                "storageKey": null
              },
              (v17/*:: as any*/)
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
      (v3/*:: as any*/),
      {
        "alias": null,
        "args": (v4/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v5/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v6/*:: as any*/),
              (v7/*:: as any*/),
              (v8/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "createdBy",
                "plural": false,
                "selections": (v19/*:: as any*/),
                "storageKey": null
              },
              (v11/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "updatedBy",
                "plural": false,
                "selections": (v19/*:: as any*/),
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
                  (v12/*:: as any*/),
                  (v16/*:: as any*/),
                  (v18/*:: as any*/)
                ],
                "storageKey": null
              },
              (v17/*:: as any*/)
            ],
            "type": "Reservation",
            "abstractKey": null
          },
          (v18/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "05cd037437ee4d6a93f49c1201e8eb2e",
    "id": null,
    "metadata": {},
    "name": "ReservationSettingsDialogQuery",
    "operationKind": "query",
    "text": "query ReservationSettingsDialogQuery(\n  $id: ID!\n) {\n  systemSettings {\n    recordingDefaults {\n      priority\n      joinSegments\n    }\n  }\n  node(id: $id) {\n    __typename\n    ... on Reservation {\n      kind\n      followEventRelay\n      createdAt\n      createdBy {\n        displayName\n        id\n      }\n      updatedAt\n      updatedBy {\n        displayName\n        id\n      }\n      rules {\n        name\n        settings {\n          startMarginSeconds\n          endMarginSeconds\n          fileNameTemplate\n          priority\n          joinSegments\n        }\n        id\n      }\n      settings {\n        poolId\n        fileNameTemplate\n        startMarginSeconds\n        endMarginSeconds\n        priority\n        joinSegments\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "364f509924d03ada7041053091b1c92e";

export default node;
