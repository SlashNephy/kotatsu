/**
 * @generated SignedSource<<e79900bd575ff4e74664253ba3b8e3c2>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CaptionModelKind = "VAD" | "WHISPER" | "%future added value";
export type CaptionModelState = "DOWNLOADING" | "FAILED" | "NOT_PLACED" | "PLACED" | "%future added value";
export type CaptionModelsQuery$variables = Record<PropertyKey, never>;
export type CaptionModelsQuery$data = {
  readonly captionModels: ReadonlyArray<{
    readonly download: {
      readonly failed: boolean;
      readonly model: {
        readonly id: string;
        readonly name: string;
      } | null | undefined;
      readonly progress: {
        readonly ratio: number;
        readonly remainingSeconds: number | null | undefined;
      } | null | undefined;
      readonly running: boolean;
    } | null | undefined;
    readonly installed: {
      readonly installedAt: string | null | undefined;
      readonly model: {
        readonly id: string;
        readonly name: string;
      } | null | undefined;
      readonly size: number;
    } | null | undefined;
    readonly kind: CaptionModelKind;
    readonly models: ReadonlyArray<{
      readonly id: string;
      readonly license: string;
      readonly name: string;
      readonly recommended: boolean;
      readonly size: number;
    }>;
    readonly overridden: boolean;
    readonly state: CaptionModelState;
  }>;
};
export type CaptionModelsQuery = {
  response: CaptionModelsQuery$data;
  variables: CaptionModelsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
},
v1 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
},
v2 = {
  "alias": null,
  "args": null,
  "concreteType": "CaptionModel",
  "kind": "LinkedField",
  "name": "model",
  "plural": false,
  "selections": [
    (v0/*:: as any*/),
    (v1/*:: as any*/)
  ],
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "size",
  "storageKey": null
},
v4 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "CaptionModelSlot",
    "kind": "LinkedField",
    "name": "captionModels",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "kind",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "state",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "overridden",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "InstalledCaptionModel",
        "kind": "LinkedField",
        "name": "installed",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "installedAt",
            "storageKey": null
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "CaptionModelDownload",
        "kind": "LinkedField",
        "name": "download",
        "plural": false,
        "selections": [
          (v2/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "running",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "failed",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "JobProgress",
            "kind": "LinkedField",
            "name": "progress",
            "plural": false,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "ratio",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "remainingSeconds",
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
        "concreteType": "CaptionModel",
        "kind": "LinkedField",
        "name": "models",
        "plural": true,
        "selections": [
          (v0/*:: as any*/),
          (v1/*:: as any*/),
          (v3/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "license",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "recommended",
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
    "name": "CaptionModelsQuery",
    "selections": (v4/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "CaptionModelsQuery",
    "selections": (v4/*:: as any*/)
  },
  "params": {
    "cacheID": "0a16ccb515470f957f30c99d348a0bc1",
    "id": null,
    "metadata": {},
    "name": "CaptionModelsQuery",
    "operationKind": "query",
    "text": "query CaptionModelsQuery {\n  captionModels {\n    kind\n    state\n    overridden\n    installed {\n      model {\n        id\n        name\n      }\n      size\n      installedAt\n    }\n    download {\n      model {\n        id\n        name\n      }\n      running\n      failed\n      progress {\n        ratio\n        remainingSeconds\n      }\n    }\n    models {\n      id\n      name\n      size\n      license\n      recommended\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "ae0557e331de8f432af107328fe56c52";

export default node;
