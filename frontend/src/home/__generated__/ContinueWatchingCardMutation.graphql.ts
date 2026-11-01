/**
 * @generated SignedSource<<be7e83f1327e5c2871142e571dc13ef7>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type ContinueWatchingCardMutation$variables = {
  id: string;
};
export type ContinueWatchingCardMutation$data = {
  readonly clearWatchPosition: {
    readonly id: string;
    readonly watchFrame: {
      readonly url: string;
    } | null | undefined;
    readonly watchPositionMs: number | null | undefined;
  };
};
export type ContinueWatchingCardMutation = {
  response: ContinueWatchingCardMutation$data;
  variables: ContinueWatchingCardMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      }
    ],
    "concreteType": "Recording",
    "kind": "LinkedField",
    "name": "clearWatchPosition",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "watchPositionMs",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "concreteType": "PreviewFrame",
        "kind": "LinkedField",
        "name": "watchFrame",
        "plural": false,
        "selections": [
          {
            "alias": null,
            "args": null,
            "kind": "ScalarField",
            "name": "url",
            "storageKey": null
          }
        ],
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
    "name": "ContinueWatchingCardMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "ContinueWatchingCardMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "0a52fcfa52a08c4f745f59d1718d35df",
    "id": null,
    "metadata": {},
    "name": "ContinueWatchingCardMutation",
    "operationKind": "mutation",
    "text": "mutation ContinueWatchingCardMutation(\n  $id: ID!\n) {\n  clearWatchPosition(id: $id) {\n    id\n    watchPositionMs\n    watchFrame {\n      url\n    }\n  }\n}\n"
  }
};
})();

(node as any).hash = "348869ae9d7087fd3030e6c5ad25985a";

export default node;
