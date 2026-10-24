/**
 * @generated SignedSource<<4c532bbc9a1ccad285896b2b801abb68>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ExternalStreamURLMutation$variables = {
  target: string;
};
export type ExternalStreamURLMutation$data = {
  readonly createStreamURL: {
    readonly expiresAt: string;
    readonly url: string;
  };
};
export type ExternalStreamURLMutation = {
  response: ExternalStreamURLMutation$data;
  variables: ExternalStreamURLMutation$variables;
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
    "name": "ExternalStreamURLMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ExternalStreamURLMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "969eb1ea6ff9b6da4f3e4e956e3e5930",
    "id": null,
    "metadata": {},
    "name": "ExternalStreamURLMutation",
    "operationKind": "mutation",
    "text": "mutation ExternalStreamURLMutation(\n  $target: ID!\n) {\n  createStreamURL(target: $target) {\n    url\n    expiresAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "f065a8554d7608d70c34151077f45897";

export default node;
