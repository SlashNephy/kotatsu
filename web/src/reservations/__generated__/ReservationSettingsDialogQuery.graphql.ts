/**
 * @generated SignedSource<<4e7d0c812e064e19016618bc0638efb7>>
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
    readonly kind: ReservationKind;
    readonly rules: ReadonlyArray<{
      readonly name: string;
      readonly settings: {
        readonly endMarginSeconds: number | null | undefined;
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
v1 = [
  {
    "kind": "Variable",
    "name": "id",
    "variableName": "id"
  }
],
v2 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "__typename",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "kind",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "createdAt",
  "storageKey": null
},
v5 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "displayName",
  "storageKey": null
},
v6 = [
  (v5/*:: as any*/)
],
v7 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "updatedAt",
  "storageKey": null
},
v8 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v9 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "startMarginSeconds",
  "storageKey": null
},
v10 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "endMarginSeconds",
  "storageKey": null
},
v11 = {
  "alias": null,
  "args": null,
  "concreteType": "RecordingSettingsOverrides",
  "kind": "LinkedField",
  "name": "settings",
  "plural": false,
  "selections": [
    (v9/*:: as any*/),
    (v10/*:: as any*/)
  ],
  "storageKey": null
},
v12 = {
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
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "fileNameTemplate",
      "storageKey": null
    },
    (v9/*:: as any*/),
    (v10/*:: as any*/),
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "priority",
      "storageKey": null
    },
    {
      "alias": null,
      "args": null,
      "kind": "ScalarField",
      "name": "joinSegments",
      "storageKey": null
    }
  ],
  "storageKey": null
},
v13 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v14 = [
  (v5/*:: as any*/),
  (v13/*:: as any*/)
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "ReservationSettingsDialogQuery",
    "selections": [
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v3/*:: as any*/),
              (v4/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "createdBy",
                "plural": false,
                "selections": (v6/*:: as any*/),
                "storageKey": null
              },
              (v7/*:: as any*/),
              {
                "alias": null,
                "args": null,
                "concreteType": "User",
                "kind": "LinkedField",
                "name": "updatedBy",
                "plural": false,
                "selections": (v6/*:: as any*/),
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
                  (v8/*:: as any*/),
                  (v11/*:: as any*/)
                ],
                "storageKey": null
              },
              (v12/*:: as any*/)
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
      {
        "alias": null,
        "args": (v1/*:: as any*/),
        "concreteType": null,
        "kind": "LinkedField",
        "name": "node",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          {
            "kind": "InlineFragment",
            "selections": [
              (v3/*:: as any*/),
              (v4/*:: as any*/),
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
              (v7/*:: as any*/),
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
                  (v8/*:: as any*/),
                  (v11/*:: as any*/),
                  (v13/*:: as any*/)
                ],
                "storageKey": null
              },
              (v12/*:: as any*/)
            ],
            "type": "Reservation",
            "abstractKey": null
          },
          (v13/*:: as any*/)
        ],
        "storageKey": null
      }
    ]
  },
  "params": {
    "cacheID": "365bd1029d0f64f6b7fe40a3fa01af27",
    "id": null,
    "metadata": {},
    "name": "ReservationSettingsDialogQuery",
    "operationKind": "query",
    "text": "query ReservationSettingsDialogQuery(\n  $id: ID!\n) {\n  node(id: $id) {\n    __typename\n    ... on Reservation {\n      kind\n      createdAt\n      createdBy {\n        displayName\n        id\n      }\n      updatedAt\n      updatedBy {\n        displayName\n        id\n      }\n      rules {\n        name\n        settings {\n          startMarginSeconds\n          endMarginSeconds\n        }\n        id\n      }\n      settings {\n        poolId\n        fileNameTemplate\n        startMarginSeconds\n        endMarginSeconds\n        priority\n        joinSegments\n      }\n    }\n    id\n  }\n}\n"
  }
};
})();

(node as any).hash = "9d31ca193e8f9639c96a94c642eb8e75";

export default node;
