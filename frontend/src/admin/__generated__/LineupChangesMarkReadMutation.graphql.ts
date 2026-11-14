/**
 * @generated SignedSource<<0670b8030dad248f764b858ab26b16b6>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type LineupChangesMarkReadMutation$variables = {
  ids: ReadonlyArray<string>;
};
export type LineupChangesMarkReadMutation$data = {
  readonly markNotificationsRead: number;
};
export type LineupChangesMarkReadMutation = {
  response: LineupChangesMarkReadMutation$data;
  variables: LineupChangesMarkReadMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "ids"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "ids",
        "variableName": "ids"
      }
    ],
    "kind": "ScalarField",
    "name": "markNotificationsRead",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "LineupChangesMarkReadMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "LineupChangesMarkReadMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "65ccd57dac7146b71e3586ef1b8de486",
    "id": null,
    "metadata": {},
    "name": "LineupChangesMarkReadMutation",
    "operationKind": "mutation",
    "text": "mutation LineupChangesMarkReadMutation(\n  $ids: [ID!]!\n) {\n  markNotificationsRead(ids: $ids)\n}\n"
  }
};
})();

(node as any).hash = "db71f3f18a38f3bab473f5260322789c";

export default node;
