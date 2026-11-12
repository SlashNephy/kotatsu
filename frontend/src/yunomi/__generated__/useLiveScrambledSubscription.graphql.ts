/**
 * @generated SignedSource<<6e5994080acfd697f5d207676e3f5d61>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type useLiveScrambledSubscription$variables = {
  service: string;
};
export type useLiveScrambledSubscription$data = {
  readonly liveStreamScrambled: {
    readonly decidedAt: string;
  };
};
export type useLiveScrambledSubscription = {
  response: useLiveScrambledSubscription$data;
  variables: useLiveScrambledSubscription$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "service"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "service",
        "variableName": "service"
      }
    ],
    "concreteType": "LiveStreamScrambled",
    "kind": "LinkedField",
    "name": "liveStreamScrambled",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "decidedAt",
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
    "name": "useLiveScrambledSubscription",
    "selections": (v1/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "useLiveScrambledSubscription",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "44aabc41f45e799739a5b79103a3a2b7",
    "id": null,
    "metadata": {},
    "name": "useLiveScrambledSubscription",
    "operationKind": "subscription",
    "text": "subscription useLiveScrambledSubscription(\n  $service: ID!\n) {\n  liveStreamScrambled(service: $service) {\n    decidedAt\n  }\n}\n"
  }
};
})();

(node as any).hash = "f4703e59fd72550a2d847c04a9213282";

export default node;
