/**
 * @generated SignedSource<<88b34d21314a41c4e34ffb2cb691ed5a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type aboutQuery$variables = Record<PropertyKey, never>;
export type aboutQuery$data = {
  readonly goModuleLicenses: ReadonlyArray<{
    readonly files: ReadonlyArray<{
      readonly name: string;
      readonly text: string;
    }>;
    readonly path: string;
    readonly version: string;
  }> | null | undefined;
  readonly version: {
    readonly revision: string | null | undefined;
    readonly version: string;
  };
};
export type aboutQuery = {
  response: aboutQuery$data;
  variables: aboutQuery$variables;
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
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "revision",
        "storageKey": null
      }
    ],
    "storageKey": null
  },
  {
    "alias": null,
    "args": null,
    "concreteType": "GoModuleLicense",
    "kind": "LinkedField",
    "name": "goModuleLicenses",
    "plural": true,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "path",
        "storageKey": null
      },
      (v0/*:: as any*/),
      {
        "alias": null,
        "args": null,
        "concreteType": "LicenseFile",
        "kind": "LinkedField",
        "name": "files",
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
            "name": "text",
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
    "name": "aboutQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "aboutQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "ef560f2a62b404e32cdd6aebcba57500",
    "id": null,
    "metadata": {},
    "name": "aboutQuery",
    "operationKind": "query",
    "text": "query aboutQuery {\n  version {\n    version\n    revision\n  }\n  goModuleLicenses {\n    path\n    version\n    files {\n      name\n      text\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "b30d0785ecb5600308db870c35ba41c5";

export default node;
