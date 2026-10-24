/**
 * @generated SignedSource<<ca1943a843bbf80622c1cdd66fa9b4b1>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type sessionsRevokeOthersMutation$variables = Record<PropertyKey, never>;
export type sessionsRevokeOthersMutation$data = {
  readonly revokeOtherSessions: number;
};
export type sessionsRevokeOthersMutation = {
  response: sessionsRevokeOthersMutation$data;
  variables: sessionsRevokeOthersMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "revokeOtherSessions",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "sessionsRevokeOthersMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "sessionsRevokeOthersMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "b8c5a6c1739db9316a676098161b41d8",
    "id": null,
    "metadata": {},
    "name": "sessionsRevokeOthersMutation",
    "operationKind": "mutation",
    "text": "mutation sessionsRevokeOthersMutation {\n  revokeOtherSessions\n}\n"
  }
};
})();

(node as any).hash = "9cb5fa58d2524c66b134d7512127ae7a";

export default node;
