/**
 * @generated SignedSource<<888b8fbf7e436bd216a163f0804f50ea>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LogLevel = "ERROR" | "INFO" | "WARN" | "%future added value";
export type LogSourceFailure = "HTTP_ERROR" | "TIMEOUT" | "UNREACHABLE" | "UNSUPPORTED" | "%future added value";
export type LogSourceKind = "KOTATSU" | "TUNER_SERVER" | "%future added value";
export type logsQuery$variables = Record<PropertyKey, never>;
export type logsQuery$data = {
  readonly logReport: {
    readonly instanceId: string;
    readonly redactHosts: ReadonlyArray<string>;
    readonly roles: ReadonlyArray<string>;
    readonly sources: ReadonlyArray<{
      readonly arch: string;
      readonly failure: LogSourceFailure | null | undefined;
      readonly kind: LogSourceKind;
      readonly lines: ReadonlyArray<{
        readonly inferred: boolean;
        readonly level: LogLevel;
        readonly text: string;
        readonly time: number;
      }>;
      readonly os: string;
      readonly runtime: string;
      readonly total: number;
      readonly version: string;
    }>;
  };
  readonly systemStatus: {
    readonly tuners: ReadonlyArray<{
      readonly name: string;
      readonly types: ReadonlyArray<string>;
    }>;
  };
  readonly version: {
    readonly version: string;
  };
};
export type logsQuery = {
  response: logsQuery$data;
  variables: logsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "version",
  "storageKey": null
},
v1 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "AppVersion",
    "kind": "LinkedField",
    "name": "version",
    "plural": false,
    "selections": [
      (v0/*:: as any*/)
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "LogReport",
    "kind": "LinkedField",
    "name": "logReport",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "LogSource",
        "kind": "LinkedField",
        "name": "sources",
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
            "name": "total",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "failure",
            "storageKey": null
          },
          (v0/*:: as any*/),
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "os",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "arch",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "runtime",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "concreteType": "LogLine",
            "kind": "LinkedField",
            "name": "lines",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "time",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "level",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "inferred",
                "storageKey": null
              },
              {
                "alias": null,
                "args": null,
                "kind": "ScalarField",
                "name": "text",
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
        "kind": "ScalarField",
        "name": "redactHosts",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "instanceId",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "roles",
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
        "concreteType": "Tuner",
        "kind": "LinkedField",
        "name": "tuners",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "name",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "types",
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
    "name": "logsQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "logsQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f9911c1366dd5818e69daa515408944b",
    "id": null,
    "metadata": {},
    "name": "logsQuery",
    "operationKind": "query",
    "text": "query logsQuery {\n  version {\n    version\n  }\n  logReport {\n    sources {\n      kind\n      total\n      failure\n      version\n      os\n      arch\n      runtime\n      lines {\n        time\n        level\n        inferred\n        text\n      }\n    }\n    redactHosts\n    instanceId\n    roles\n  }\n  systemStatus {\n    tuners {\n      name\n      types\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "9a4e79d06f7c78a2c79eaa8b7cb7ff4d";

export default node;
