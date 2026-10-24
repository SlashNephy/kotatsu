/**
 * @generated SignedSource<<09efedd2f27cecd0447b0d5c39662a63>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type FileNameTemplateFieldPreviewQuery$variables = {
  program?: string | null | undefined;
  template: string;
};
export type FileNameTemplateFieldPreviewQuery$data = {
  readonly previewFileName: {
    readonly error: string | null | undefined;
    readonly fileName: string | null | undefined;
  };
};
export type FileNameTemplateFieldPreviewQuery = {
  response: FileNameTemplateFieldPreviewQuery$data;
  variables: FileNameTemplateFieldPreviewQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "program"
},
v1 = {
  "defaultValue": null,
  "kind": "LocalArgument",
  "name": "template"
},
v2 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "program",
        "variableName": "program"
      },
      {
        "kind": "Variable",
        "name": "template",
        "variableName": "template"
      }
    ],
    "concreteType": "FileNamePreview",
    "kind": "LinkedField",
    "name": "previewFileName",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "fileName",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "error",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [
      (v0/*:: as any*/),
      (v1/*:: as any*/)
    ],
    "kind": "Fragment",
    "metadata": null,
    "name": "FileNameTemplateFieldPreviewQuery",
    "selections": (v2/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [
      (v1/*:: as any*/),
      (v0/*:: as any*/)
    ],
    "kind": "Operation",
    "name": "FileNameTemplateFieldPreviewQuery",
    "selections": (v2/*:: as any*/)
  },
  "params": {
    "cacheID": "ad573cb71885b007ebf2b1786477b2e2",
    "id": null,
    "metadata": {},
    "name": "FileNameTemplateFieldPreviewQuery",
    "operationKind": "query",
    "text": "query FileNameTemplateFieldPreviewQuery(\n  $template: String!\n  $program: ID\n) {\n  previewFileName(template: $template, program: $program) {\n    fileName\n    error\n  }\n}\n"
  }
};
})();

(node as any).hash = "3339da466c6c3a1371785a44dd919949";

export default node;
