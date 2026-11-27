/**
 * @generated SignedSource<<d3a099f59a34884c9398c9fa2b7dcde0>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type captionsBackfillMutation$variables = Record<PropertyKey, never>;
export type captionsBackfillMutation$data = {
  readonly extractARIBCaptions: number;
};
export type captionsBackfillMutation = {
  response: captionsBackfillMutation$data;
  variables: captionsBackfillMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "extractARIBCaptions",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "captionsBackfillMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "captionsBackfillMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "fabca470a3ce0765e1f919dce9b2dca0",
    "id": null,
    "metadata": {},
    "name": "captionsBackfillMutation",
    "operationKind": "mutation",
    "text": "mutation captionsBackfillMutation {\n  extractARIBCaptions\n}\n"
  }
};
})();

(node as any).hash = "71917832100ca5af81916e1096fb0ae5";

export default node;
