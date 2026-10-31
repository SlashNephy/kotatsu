/**
 * @generated SignedSource<<3f281622584c215d0b92ad37dd11cf76>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type StreamInfoLiveRatesSubscription$variables = {
  service: string;
};
export type StreamInfoLiveRatesSubscription$data = {
  readonly liveStreamRates: {
    readonly audioBps: number;
    readonly drops: number;
    readonly errors: number;
    readonly otherBps: number;
    readonly psiBps: number;
    readonly scrambled: number;
    readonly totalBps: number;
    readonly unknownBps: number;
    readonly videoBps: number;
  };
};
export type StreamInfoLiveRatesSubscription = {
  response: StreamInfoLiveRatesSubscription$data;
  variables: StreamInfoLiveRatesSubscription$variables;
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
    "concreteType": "LiveStreamRates",
    "kind": "LinkedField",
    "name": "liveStreamRates",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "videoBps",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "audioBps",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "otherBps",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "psiBps",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "unknownBps",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "totalBps",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "drops",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "errors",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "scrambled",
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
    "name": "StreamInfoLiveRatesSubscription",
    "selections": (v1/*:: as any*/),
    "type": "Subscription",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "StreamInfoLiveRatesSubscription",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "f38e8831f0bae2d15b1a64a15194dc0c",
    "id": null,
    "metadata": {},
    "name": "StreamInfoLiveRatesSubscription",
    "operationKind": "subscription",
    "text": "subscription StreamInfoLiveRatesSubscription(\n  $service: ID!\n) {\n  liveStreamRates(service: $service) {\n    videoBps\n    audioBps\n    otherBps\n    psiBps\n    unknownBps\n    totalBps\n    drops\n    errors\n    scrambled\n  }\n}\n"
  }
};
})();

(node as any).hash = "5413ab4bf8abf4781f491233100d4759";

export default node;
