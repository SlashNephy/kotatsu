/**
 * @generated SignedSource<<954220360d98744a817e98baf81e7557>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ServiceIdRecordingsQuery$variables = Record<PropertyKey, never>;
export type ServiceIdRecordingsQuery$data = {
  readonly recordings: {
    readonly edges: ReadonlyArray<{
      readonly node: {
        readonly name: string;
        readonly recordedFrom: string;
        readonly service: {
          readonly logoUrl: string | null | undefined;
          readonly name: string;
        };
      };
    }>;
  };
};
export type ServiceIdRecordingsQuery = {
  response: ServiceIdRecordingsQuery$data;
  variables: ServiceIdRecordingsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "kind": "Literal",
    "name": "filter",
    "value": {
      "states": [
        "RECORDING"
      ]
    }
  },
  {
    "kind": "Literal",
    "name": "first",
    "value": 20
  }
],
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
  "kind": "ScalarField",
  "name": "recordedFrom",
  "storageKey": null
},
v3 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "logoUrl",
  "storageKey": null
},
v4 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "id",
  "storageKey": null
};
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "ServiceIdRecordingsQuery",
    "selections": [
      {
        "alias": null,
        "args": (v0/*:: as any*/),
        "concreteType": "RecordingConnection",
        "kind": "LinkedField",
        "name": "recordings",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "RecordingEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Recording",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v1/*:: as any*/),
                  (v2/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v1/*:: as any*/),
                      (v3/*:: as any*/)
                    ],
                    "storageKey": null
                  }
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": "recordings(filter:{\"states\":[\"RECORDING\"]},first:20)"
      }
    ],
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "ServiceIdRecordingsQuery",
    "selections": [
      {
        "alias": null,
        "args": (v0/*:: as any*/),
        "concreteType": "RecordingConnection",
        "kind": "LinkedField",
        "name": "recordings",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "concreteType": "RecordingEdge",
            "kind": "LinkedField",
            "name": "edges",
            "plural": true,
            "selections": [
              {
                "alias": null,
                "args": null,
                "concreteType": "Recording",
                "kind": "LinkedField",
                "name": "node",
                "plural": false,
                "selections": [
                  (v1/*:: as any*/),
                  (v2/*:: as any*/),
                  {
                    "alias": null,
                    "args": null,
                    "concreteType": "Service",
                    "kind": "LinkedField",
                    "name": "service",
                    "plural": false,
                    "selections": [
                      (v1/*:: as any*/),
                      (v3/*:: as any*/),
                      (v4/*:: as any*/)
                    ],
                    "storageKey": null
                  },
                  (v4/*:: as any*/)
                ],
                "storageKey": null
              }
            ],
            "storageKey": null
          }
        ],
        "storageKey": "recordings(filter:{\"states\":[\"RECORDING\"]},first:20)"
      }
    ]
  },
  "params": {
    "cacheID": "b16bac86c8d05e22b48ed96a97ab52c0",
    "id": null,
    "metadata": {},
    "name": "ServiceIdRecordingsQuery",
    "operationKind": "query",
    "text": "query ServiceIdRecordingsQuery {\n  recordings(first: 20, filter: {states: [RECORDING]}) {\n    edges {\n      node {\n        name\n        recordedFrom\n        service {\n          name\n          logoUrl\n          id\n        }\n        id\n      }\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "f182171d6184ec2022962bc2cdf41bc3";

export default node;
