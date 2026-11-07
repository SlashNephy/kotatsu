/**
 * @generated SignedSource<<9069067435d103e0a98d89259cefff1a>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type IptvFeedDeleteMutation$variables = Record<PropertyKey, never>;
export type IptvFeedDeleteMutation$data = {
  readonly deleteIptvFeed: boolean;
};
export type IptvFeedDeleteMutation = {
  response: IptvFeedDeleteMutation$data;
  variables: IptvFeedDeleteMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "deleteIptvFeed",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "IptvFeedDeleteMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "IptvFeedDeleteMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "e1eb259520afcdac1e433c056ef1dce2",
    "id": null,
    "metadata": {},
    "name": "IptvFeedDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation IptvFeedDeleteMutation {\n  deleteIptvFeed\n}\n"
  }
};
})();

(node as any).hash = "c1e19c9ca872704a0e74d542b530fe83";

export default node;
