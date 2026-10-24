/**
 * @generated SignedSource<<720cdebb59644de019088d1ca4415d87>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type FileNameTemplateFieldDefaultQuery$variables = Record<PropertyKey, never>;
export type FileNameTemplateFieldDefaultQuery$data = {
  readonly systemSettings: {
    readonly recordingDefaults: {
      readonly fileNameTemplate: string;
    };
  };
};
export type FileNameTemplateFieldDefaultQuery = {
  response: FileNameTemplateFieldDefaultQuery$data;
  variables: FileNameTemplateFieldDefaultQuery$variables;
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
            "name": "fileNameTemplate",
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
    "name": "FileNameTemplateFieldDefaultQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "FileNameTemplateFieldDefaultQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "9f718699040b248f36e0da44c027722c",
    "id": null,
    "metadata": {},
    "name": "FileNameTemplateFieldDefaultQuery",
    "operationKind": "query",
    "text": "query FileNameTemplateFieldDefaultQuery {\n  systemSettings {\n    recordingDefaults {\n      fileNameTemplate\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "8dd1e60e279cb17f31652c26baa63901";

export default node;
