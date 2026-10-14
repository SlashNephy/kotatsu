/**
 * @generated SignedSource<<600c3a0b0adcc5ed6d38fce365419ef5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type FileNameTemplateFieldHelpQuery$variables = Record<PropertyKey, never>;
export type FileNameTemplateFieldHelpQuery$data = {
  readonly fileNameTemplateHelp: {
    readonly items: ReadonlyArray<{
      readonly description: string;
      readonly example: string;
      readonly snippet: string;
    }>;
    readonly presets: ReadonlyArray<{
      readonly example: string;
      readonly name: string;
      readonly template: string;
    }>;
  };
};
export type FileNameTemplateFieldHelpQuery = {
  response: FileNameTemplateFieldHelpQuery$data;
  variables: FileNameTemplateFieldHelpQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "example",
  "storageKey": null
},
v1 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "FileNameTemplateHelp",
    "kind": "LinkedField",
    "name": "fileNameTemplateHelp",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "FileNameTemplateItem",
        "kind": "LinkedField",
        "name": "items",
        "plural": true,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "snippet",
            "storageKey": null
          },
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "description",
            "storageKey": null
          },
          (v0/*:: as any*/)
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "FileNameTemplatePreset",
        "kind": "LinkedField",
        "name": "presets",
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
            "name": "template",
            "storageKey": null
          },
          (v0/*:: as any*/)
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
    "name": "FileNameTemplateFieldHelpQuery",
    "selections": (v1/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "FileNameTemplateFieldHelpQuery",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "c066b84c71012c062e84d64da0ca7e90",
    "id": null,
    "metadata": {},
    "name": "FileNameTemplateFieldHelpQuery",
    "operationKind": "query",
    "text": "query FileNameTemplateFieldHelpQuery {\n  fileNameTemplateHelp {\n    items {\n      snippet\n      description\n      example\n    }\n    presets {\n      name\n      template\n      example\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "8d205550c7b82f2fca2d73a7955f9276";

export default node;
