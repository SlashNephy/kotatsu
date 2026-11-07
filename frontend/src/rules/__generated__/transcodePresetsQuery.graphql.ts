/**
 * @generated SignedSource<<38c99870586c356cd55e453809aeed9a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type transcodePresetsQuery$variables = Record<PropertyKey, never>;
export type transcodePresetsQuery$data = {
  readonly transcodePresets: ReadonlyArray<string>;
};
export type transcodePresetsQuery = {
  response: transcodePresetsQuery$data;
  variables: transcodePresetsQuery$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "transcodePresets",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "transcodePresetsQuery",
    "selections": (v0/*:: as any*/),
    "type": "Query",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "transcodePresetsQuery",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "30373b5b439ade48d0ca080c1cf1d1d0",
    "id": null,
    "metadata": {},
    "name": "transcodePresetsQuery",
    "operationKind": "query",
    "text": "query transcodePresetsQuery {\n  transcodePresets\n}\n"
  }
};
})();

(node as any).hash = "40689efe789af3b93c135be5d68c2964";

export default node;
