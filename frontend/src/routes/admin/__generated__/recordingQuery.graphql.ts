/**
 * @generated SignedSource<<b290994201d2230163c1e35e0e5d8cc4>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type recordingQuery$variables = Record<PropertyKey, never>;
export type recordingQuery$data = {
  readonly systemSettings: {
    readonly recordingDefaults: {
      readonly detectCm: boolean;
      readonly endMarginSeconds: number;
      readonly fileNameTemplate: string;
      readonly joinSegments: boolean;
      readonly poolId: string | null | undefined;
      readonly priority: number;
      readonly startMarginSeconds: number;
      readonly transcodePreset: string | null | undefined;
    };
    readonly ruleDefaults: {
      readonly avoidDuplicates: boolean;
      readonly duplicateWindowDays: number | null | undefined;
      readonly followEventRelay: boolean;
    };
    readonly updatedAt: string | null | undefined;
  };
  readonly systemStatus: {
    readonly pools: ReadonlyArray<{
      readonly id: string;
    }>;
  };
};
export type recordingQuery = {
  response: recordingQuery$data;
  variables: recordingQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
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
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "startMarginSeconds",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "endMarginSeconds",
            "storageKey": null
          },
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
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "detectCm",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "transcodePreset",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "RuleDefaults",
        "kind": "LinkedField",
        "name": "ruleDefaults",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "avoidDuplicates",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "duplicateWindowDays",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "followEventRelay",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "updatedAt",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "SystemStatus",
    "kind": "LinkedField",
    "name": "systemStatus",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "StoragePool",
        "kind": "LinkedField",
        "name": "pools",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "id",
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
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "recordingQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "recordingQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "a5b0368ec1998d91956708b2a620cb8f",
    "id": null,
    "metadata": {},
    "name": "recordingQuery",
    "operationKind": "query",
    "text": "query recordingQuery {\n  systemSettings {\n    recordingDefaults {\n      poolId\n      fileNameTemplate\n      startMarginSeconds\n      endMarginSeconds\n      priority\n      joinSegments\n      detectCm\n      transcodePreset\n    }\n    ruleDefaults {\n      avoidDuplicates\n      duplicateWindowDays\n      followEventRelay\n    }\n    updatedAt\n  }\n  systemStatus {\n    pools {\n      id\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "f2eaa3e965863f717a91e7990011ab72";

export default node;
