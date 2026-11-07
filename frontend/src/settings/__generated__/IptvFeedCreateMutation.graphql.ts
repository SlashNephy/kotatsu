/**
 * @generated SignedSource<<f842146dc9dbd9e4854dbc85af9a89c7>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type IptvFeedCreateMutation$variables = Record<PropertyKey, never>;
export type IptvFeedCreateMutation$data = {
  readonly createIptvFeed: {
    readonly guidePath: string;
    readonly playlistPath: string;
  };
};
export type IptvFeedCreateMutation = {
  response: IptvFeedCreateMutation$data;
  variables: IptvFeedCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "concreteType": "CreatedIptvFeed",
    "kind": "LinkedField",
    "name": "createIptvFeed",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "playlistPath",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "guidePath",
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
    "name": "IptvFeedCreateMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "IptvFeedCreateMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "d2508348e7011eda563b49240a7b17cf",
    "id": null,
    "metadata": {},
    "name": "IptvFeedCreateMutation",
    "operationKind": "mutation",
    "text": "mutation IptvFeedCreateMutation {\n  createIptvFeed {\n    playlistPath\n    guidePath\n  }\n}\n"
  }
};
})();

(node as any).hash = "3e390ba8583db7c114310951e6e6a110";

export default node;
