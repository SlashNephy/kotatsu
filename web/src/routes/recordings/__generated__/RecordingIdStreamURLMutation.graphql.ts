/**
 * @generated SignedSource<<420e77147155b671a1824866f73901f8>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type RecordingIdStreamURLMutation$variables = {
  target: string;
};
export type RecordingIdStreamURLMutation$data = {
  readonly createStreamURL: {
    readonly expiresAt: string;
    readonly url: string;
  };
};
export type RecordingIdStreamURLMutation = {
  response: RecordingIdStreamURLMutation$data;
  variables: RecordingIdStreamURLMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "target"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "target",
        "variableName": "target"
      }
    ],
    "concreteType": "StreamURL",
    "kind": "LinkedField",
    "name": "createStreamURL",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "url",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "expiresAt",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "RecordingIdStreamURLMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "RecordingIdStreamURLMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "2658ec04412cc3ddbbb1860651e61187",
    "id": null,
    "metadata": {},
    "name": "RecordingIdStreamURLMutation",
    "operationKind": "mutation",
    "text": "mutation RecordingIdStreamURLMutation(\n  $target: ID!\n) {\n  createStreamURL(target: $target) {\n    url\n    expiresAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "d53d72fc993a207a6dc27d8a5884108f";

export default node;
